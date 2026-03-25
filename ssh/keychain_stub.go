//go:build !darwin || !cgo

package ssh

import "fmt"

// KeychainStore is a stub for platforms without macOS Keychain support.
type KeychainStore struct {
	service string
}

func NewKeychainStore(service string) *KeychainStore {
	return &KeychainStore{service: service}
}

func (k *KeychainStore) Set(hostID string, secret string) error {
	return fmt.Errorf("keychain not available: requires darwin with cgo")
}

func (k *KeychainStore) Get(hostID string) (string, error) {
	return "", fmt.Errorf("keychain not available: requires darwin with cgo")
}

func (k *KeychainStore) Delete(hostID string) error {
	return fmt.Errorf("keychain not available: requires darwin with cgo")
}
