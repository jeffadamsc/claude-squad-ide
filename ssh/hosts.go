package ssh

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

const (
	AuthMethodPassword      = "password"
	AuthMethodKey           = "key"
	AuthMethodKeyPassphrase = "key+passphrase"
)

// HostConfig is a saved SSH host configuration. Secrets (passwords, passphrases)
// are stored in the macOS Keychain, not here.
type HostConfig struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	AuthMethod string `json:"authMethod"`
	KeyPath    string `json:"keyPath,omitempty"`
	LastPath   string `json:"lastPath,omitempty"`
}

// HostStore manages persistent host configurations in a JSON file.
type HostStore struct {
	mu   sync.Mutex
	path string
}

func NewHostStore(path string) *HostStore {
	return &HostStore{path: path}
}

func (s *HostStore) LoadAll() ([]HostConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *HostStore) loadLocked() ([]HostConfig, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []HostConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read hosts file: %w", err)
	}
	var hosts []HostConfig
	if err := json.Unmarshal(data, &hosts); err != nil {
		return nil, fmt.Errorf("parse hosts file: %w", err)
	}
	return hosts, nil
}

func (s *HostStore) saveLocked(hosts []HostConfig) error {
	data, err := json.MarshalIndent(hosts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal hosts: %w", err)
	}
	return os.WriteFile(s.path, data, 0600)
}

func (s *HostStore) Save(host HostConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	hosts, err := s.loadLocked()
	if err != nil {
		return err
	}
	hosts = append(hosts, host)
	return s.saveLocked(hosts)
}

func (s *HostStore) Update(host HostConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	hosts, err := s.loadLocked()
	if err != nil {
		return err
	}
	for i, h := range hosts {
		if h.ID == host.ID {
			hosts[i] = host
			return s.saveLocked(hosts)
		}
	}
	return fmt.Errorf("host %s not found", host.ID)
}

func (s *HostStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	hosts, err := s.loadLocked()
	if err != nil {
		return err
	}
	filtered := make([]HostConfig, 0, len(hosts))
	for _, h := range hosts {
		if h.ID != id {
			filtered = append(filtered, h)
		}
	}
	return s.saveLocked(filtered)
}

func (s *HostStore) GetByID(id string) (HostConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hosts, err := s.loadLocked()
	if err != nil {
		return HostConfig{}, err
	}
	for _, h := range hosts {
		if h.ID == id {
			return h, nil
		}
	}
	return HostConfig{}, fmt.Errorf("host %s not found", id)
}
