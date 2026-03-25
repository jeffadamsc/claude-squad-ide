//go:build darwin && cgo

package ssh

import (
	"fmt"

	"github.com/keybase/go-keychain"
)

// KeychainStore manages secrets in the macOS Keychain.
type KeychainStore struct {
	service string
}

func NewKeychainStore(service string) *KeychainStore {
	return &KeychainStore{service: service}
}

// Set stores or updates a secret for the given host ID.
func (k *KeychainStore) Set(hostID string, secret string) error {
	// Try to delete existing item first (update case)
	_ = k.Delete(hostID)

	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(k.service)
	item.SetAccount(hostID)
	item.SetData([]byte(secret))
	item.SetSynchronizable(keychain.SynchronizableNo)
	item.SetAccessible(keychain.AccessibleWhenUnlocked)

	return keychain.AddItem(item)
}

// Get retrieves the secret for the given host ID.
func (k *KeychainStore) Get(hostID string) (string, error) {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(k.service)
	query.SetAccount(hostID)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnData(true)

	results, err := keychain.QueryItem(query)
	if err != nil {
		return "", fmt.Errorf("keychain query: %w", err)
	}
	if len(results) == 0 {
		return "", fmt.Errorf("no secret found for host %s", hostID)
	}
	return string(results[0].Data), nil
}

// Delete removes the secret for the given host ID.
func (k *KeychainStore) Delete(hostID string) error {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(k.service)
	item.SetAccount(hostID)
	return keychain.DeleteItem(item)
}
