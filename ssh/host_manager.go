package ssh

import (
	"fmt"
	"sync"
	"time"

	"claude-squad/log"
)

const idleTimeout = 30 * time.Second

// HostManager manages SSH connections, one per host, shared across sessions.
type HostManager struct {
	mu       sync.Mutex
	store    *HostStore
	keychain *KeychainStore
	clients  map[string]*managedClient
}

type managedClient struct {
	client    *Client
	pm        *SSHProcessManager
	refCount  int
	idleTimer *time.Timer
}

func NewHostManager(store *HostStore, keychain *KeychainStore) *HostManager {
	return &HostManager{
		store:    store,
		keychain: keychain,
		clients:  make(map[string]*managedClient),
	}
}

// GetClient returns a connected SSH client for the host, creating one if needed.
func (hm *HostManager) GetClient(hostID string) (*Client, error) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if mc, ok := hm.clients[hostID]; ok {
		if mc.client.Connected() {
			mc.refCount++
			if mc.idleTimer != nil {
				mc.idleTimer.Stop()
				mc.idleTimer = nil
			}
			return mc.client, nil
		}
		// Connection is dead, remove it
		delete(hm.clients, hostID)
	}

	// Create new connection
	config, err := hm.store.GetByID(hostID)
	if err != nil {
		return nil, fmt.Errorf("get host config: %w", err)
	}

	var secret string
	if config.AuthMethod == AuthMethodPassword || config.AuthMethod == AuthMethodKeyPassphrase {
		secret, err = hm.keychain.Get(hostID)
		if err != nil {
			return nil, fmt.Errorf("get secret from keychain: %w", err)
		}
	}

	client := NewClient(config, secret)
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("connect to %s: %w", config.Name, err)
	}

	mc := &managedClient{
		client:   client,
		pm:       NewSSHProcessManager(client, hostID),
		refCount: 1,
	}

	// Set up reconnect callback
	client.OnDisconnect(func() {
		hm.handleDisconnect(hostID)
	})

	hm.clients[hostID] = mc
	return client, nil
}

// GetProcessManager returns the SSHProcessManager for a host.
func (hm *HostManager) GetProcessManager(hostID string) (*SSHProcessManager, error) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	mc, ok := hm.clients[hostID]
	if !ok {
		return nil, fmt.Errorf("no connection for host %s", hostID)
	}
	return mc.pm, nil
}

// ReleaseClient decrements the reference count for a host connection.
func (hm *HostManager) ReleaseClient(hostID string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	mc, ok := hm.clients[hostID]
	if !ok {
		return
	}
	mc.refCount--
	if mc.refCount <= 0 {
		mc.idleTimer = time.AfterFunc(idleTimeout, func() {
			hm.mu.Lock()
			defer hm.mu.Unlock()
			if mc2, ok := hm.clients[hostID]; ok && mc2 == mc && mc2.refCount <= 0 {
				mc2.client.Close()
				delete(hm.clients, hostID)
			}
		})
	}
}

func (hm *HostManager) handleDisconnect(hostID string) {
	log.InfoLog.Printf("SSH connection to host %s lost, starting reconnect", hostID)
	go hm.reconnectLoop(hostID)
}

func (hm *HostManager) reconnectLoop(hostID string) {
	const maxDuration = 5 * time.Minute
	delay := 3 * time.Second
	maxDelay := 30 * time.Second
	deadline := time.Now().Add(maxDuration)

	for {
		time.Sleep(delay)

		if time.Now().After(deadline) {
			log.ErrorLog.Printf("reconnect to host %s: giving up after %v", hostID, maxDuration)
			return
		}

		hm.mu.Lock()
		_, ok := hm.clients[hostID]
		hm.mu.Unlock()
		if !ok {
			return // Host was removed
		}

		config, err := hm.store.GetByID(hostID)
		if err != nil {
			log.ErrorLog.Printf("reconnect: get host config: %v", err)
			return
		}

		var secret string
		if config.AuthMethod == AuthMethodPassword || config.AuthMethod == AuthMethodKeyPassphrase {
			secret, _ = hm.keychain.Get(hostID)
		}

		newClient := NewClient(config, secret)
		if err := newClient.Connect(); err != nil {
			log.InfoLog.Printf("reconnect to %s failed: %v, retrying in %v", config.Name, err, delay)
			delay = min(delay*2, maxDelay)
			continue
		}

		// Reconnected successfully — update the client but keep the existing
		// ProcessManager so that session references (and therefore WebSocket
		// subscribers) remain valid. The old SSH sessions are dead, but the
		// ProcessManager still holds their snapshot buffers so reconnecting
		// WebSocket clients can retrieve the last terminal state.
		hm.mu.Lock()
		mc, ok := hm.clients[hostID]
		if ok {
			mc.client = newClient
			mc.pm.UpdateClient(newClient)
			newClient.OnDisconnect(func() {
				hm.handleDisconnect(hostID)
			})
		}
		hm.mu.Unlock()

		log.InfoLog.Printf("reconnected to %s", config.Name)
		return
	}
}

// GetAllProcessManagers returns all active SSHProcessManagers for registry lookup.
func (hm *HostManager) GetAllProcessManagers() []*SSHProcessManager {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	pms := make([]*SSHProcessManager, 0, len(hm.clients))
	for _, mc := range hm.clients {
		pms = append(pms, mc.pm)
	}
	return pms
}

// IsConnected returns whether the SSH connection for a host is alive.
// Read-only — does not create connections or change refcounts.
func (hm *HostManager) IsConnected(hostID string) bool {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	mc, ok := hm.clients[hostID]
	if !ok {
		return false
	}
	return mc.client.Connected()
}

// Close shuts down all managed connections.
func (hm *HostManager) Close() {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	for id, mc := range hm.clients {
		mc.client.Close()
		delete(hm.clients, id)
	}
}
