package ssh

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"claude-squad/log"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	dialTimeout       = 10 * time.Second
	keepaliveInterval = 5 * time.Second
)

// Client wraps an SSH connection with keepalive and reconnect support.
type Client struct {
	mu        sync.RWMutex
	client    *ssh.Client
	config    HostConfig
	secret    string // password or passphrase from keychain
	connected bool
	lastErr   error
	homeDir   string // cached remote $HOME

	onDisconnect  func()
	stopKeepalive chan struct{}
}

func NewClient(config HostConfig, secret string) *Client {
	return &Client{
		config: config,
		secret: secret,
	}
}

func (c *Client) address() string {
	return fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
}

// Connect establishes the SSH connection and starts keepalive.
func (c *Client) Connect() error {
	sshCfg, err := buildSSHConfig(c.config, c.secret)
	if err != nil {
		return fmt.Errorf("build ssh config: %w", err)
	}

	client, err := ssh.Dial("tcp", c.address(), sshCfg)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}

	c.mu.Lock()
	c.client = client
	c.connected = true
	c.lastErr = nil
	c.stopKeepalive = make(chan struct{})
	c.mu.Unlock()

	// Resolve remote home directory
	home, err := c.RunCommand("echo $HOME")
	if err != nil {
		client.Close()
		return fmt.Errorf("resolve remote home: %w", err)
	}
	c.mu.Lock()
	c.homeDir = strings.TrimSpace(home)
	c.mu.Unlock()

	go c.keepaliveLoop()
	return nil
}

// TestConnection tests SSH connectivity and checks if program is available.
func TestConnection(config HostConfig, secret string, program string) (connOK bool, progOK bool, errMsg string) {
	sshCfg, err := buildSSHConfig(config, secret)
	if err != nil {
		return false, false, fmt.Sprintf("Auth config error: %v", err)
	}

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return false, false, fmt.Sprintf("Connection failed: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return true, false, fmt.Sprintf("Session error: %v", err)
	}
	defer session.Close()

	// Use login shell so ~/.profile and ~/.bashrc are sourced for PATH
	progCmd := fmt.Sprintf("bash -lc 'which %s'", program)
	if err := session.Run(progCmd); err != nil {
		return true, false, fmt.Sprintf("Program '%s' not found on remote PATH", program)
	}
	return true, true, ""
}

// RunCommand executes a command on the remote host and returns combined output.
func (c *Client) RunCommand(cmd string) (string, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()
	if client == nil {
		return "", fmt.Errorf("not connected")
	}

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)
	return string(out), err
}

// Connected returns whether the SSH connection is alive.
func (c *Client) Connected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// LastError returns the last connection error.
func (c *Client) LastError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastErr
}

// HomeDir returns the cached remote home directory.
func (c *Client) HomeDir() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.homeDir
}

// SSHClient returns the underlying ssh.Client for creating sessions.
func (c *Client) SSHClient() *ssh.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client
}

// OnDisconnect registers a callback fired when the connection drops.
func (c *Client) OnDisconnect(fn func()) {
	c.mu.Lock()
	c.onDisconnect = fn
	c.mu.Unlock()
}

// Close tears down the connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopKeepalive != nil {
		close(c.stopKeepalive)
		c.stopKeepalive = nil
	}
	c.connected = false
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

func (c *Client) keepaliveLoop() {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopKeepalive:
			return
		case <-ticker.C:
			c.mu.RLock()
			client := c.client
			c.mu.RUnlock()
			if client == nil {
				return
			}
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				c.mu.Lock()
				c.connected = false
				c.lastErr = err
				cb := c.onDisconnect
				c.mu.Unlock()
				if cb != nil {
					cb()
				}
				return
			}
		}
	}
}

func buildSSHConfig(host HostConfig, secret string) (*ssh.ClientConfig, error) {
	cfg := &ssh.ClientConfig{
		User:    host.User,
		Timeout: dialTimeout,
	}

	// Host key verification via known_hosts
	knownHostsPath := os.ExpandEnv("$HOME/.ssh/known_hosts")
	if _, err := os.Stat(knownHostsPath); err == nil {
		hostKeyCallback, err := knownhosts.New(knownHostsPath)
		if err != nil {
			log.ErrorLog.Printf("failed to parse known_hosts, falling back to insecure: %v", err)
			cfg.HostKeyCallback = ssh.InsecureIgnoreHostKey()
		} else {
			cfg.HostKeyCallback = hostKeyCallback
		}
	} else {
		cfg.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	switch host.AuthMethod {
	case AuthMethodPassword:
		cfg.Auth = []ssh.AuthMethod{
			ssh.Password(secret),
			ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = secret
				}
				return answers, nil
			}),
		}
	case AuthMethodKey:
		keyData, err := os.ReadFile(host.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		cfg.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	case AuthMethodKeyPassphrase:
		keyData, err := os.ReadFile(host.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		signer, err := ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(secret))
		if err != nil {
			return nil, fmt.Errorf("parse private key with passphrase: %w", err)
		}
		cfg.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	default:
		return nil, fmt.Errorf("unsupported auth method: %s", host.AuthMethod)
	}

	return cfg, nil
}

// ShellEscape quotes a string for safe use in a shell command.
func ShellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
