//go:build darwin && cgo

package ssh

import (
	cryptoRand "crypto/rand"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestID() string {
	b := make([]byte, 4)
	_, _ = cryptoRand.Read(b)
	return fmt.Sprintf("%x", b)
}

func TestKeychainStore_SetAndGet(t *testing.T) {
	ks := NewKeychainStore("com.claude-squad.test")
	hostID := "test-kc-" + generateTestID()
	defer ks.Delete(hostID)

	err := ks.Set(hostID, "my-secret-password")
	require.NoError(t, err)

	secret, err := ks.Get(hostID)
	require.NoError(t, err)
	assert.Equal(t, "my-secret-password", secret)
}

func TestKeychainStore_GetMissing(t *testing.T) {
	ks := NewKeychainStore("com.claude-squad.test")
	_, err := ks.Get("nonexistent-host-id-" + generateTestID())
	assert.Error(t, err)
}

func TestKeychainStore_Delete(t *testing.T) {
	ks := NewKeychainStore("com.claude-squad.test")
	hostID := "test-kc-del-" + generateTestID()

	require.NoError(t, ks.Set(hostID, "temp-secret"))
	require.NoError(t, ks.Delete(hostID))

	_, err := ks.Get(hostID)
	assert.Error(t, err)
}

func TestKeychainStore_Update(t *testing.T) {
	ks := NewKeychainStore("com.claude-squad.test")
	hostID := "test-kc-upd-" + generateTestID()
	defer ks.Delete(hostID)

	require.NoError(t, ks.Set(hostID, "old-secret"))
	require.NoError(t, ks.Set(hostID, "new-secret"))

	secret, err := ks.Get(hostID)
	require.NoError(t, err)
	assert.Equal(t, "new-secret", secret)
}
