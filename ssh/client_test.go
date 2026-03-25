package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"
)

func createTestKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pemBlock, err := gossh.MarshalPrivateKey(priv, "")
	require.NoError(t, err)

	keyPath := filepath.Join(t.TempDir(), "id_test")
	err = os.WriteFile(keyPath, pem.EncodeToMemory(pemBlock), 0600)
	require.NoError(t, err)
	return keyPath
}

func createTestEncryptedKey(t *testing.T, passphrase string) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pemBlock, err := gossh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	require.NoError(t, err)

	keyPath := filepath.Join(t.TempDir(), "id_test_enc")
	err = os.WriteFile(keyPath, pem.EncodeToMemory(pemBlock), 0600)
	require.NoError(t, err)
	return keyPath
}

func TestBuildSSHConfig_Password(t *testing.T) {
	cfg, err := buildSSHConfig(HostConfig{
		User:       "deploy",
		AuthMethod: AuthMethodPassword,
	}, "my-password")
	require.NoError(t, err)
	assert.Equal(t, "deploy", cfg.User)
	assert.Len(t, cfg.Auth, 2) // password + keyboard-interactive
}

func TestBuildSSHConfig_Key(t *testing.T) {
	keyPath := createTestKey(t)
	cfg, err := buildSSHConfig(HostConfig{
		User:       "deploy",
		AuthMethod: AuthMethodKey,
		KeyPath:    keyPath,
	}, "")
	require.NoError(t, err)
	assert.Equal(t, "deploy", cfg.User)
	assert.Len(t, cfg.Auth, 1)
}

func TestBuildSSHConfig_KeyWithPassphrase(t *testing.T) {
	keyPath := createTestEncryptedKey(t, "test-passphrase")
	cfg, err := buildSSHConfig(HostConfig{
		User:       "deploy",
		AuthMethod: AuthMethodKeyPassphrase,
		KeyPath:    keyPath,
	}, "test-passphrase")
	require.NoError(t, err)
	assert.Equal(t, "deploy", cfg.User)
	assert.Len(t, cfg.Auth, 1)
}

func TestBuildSSHConfig_MissingKeyFile(t *testing.T) {
	_, err := buildSSHConfig(HostConfig{
		User:       "deploy",
		AuthMethod: AuthMethodKey,
		KeyPath:    "/nonexistent/key",
	}, "")
	assert.Error(t, err)
}

func TestClient_Address(t *testing.T) {
	c := &Client{config: HostConfig{Host: "10.0.0.1", Port: 2222}}
	assert.Equal(t, "10.0.0.1:2222", c.address())
}
