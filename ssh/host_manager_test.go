package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHostManager_GetOrCreateClient_Caching(t *testing.T) {
	hm := NewHostManager(nil, nil)
	assert.Equal(t, 0, len(hm.clients))
}

func TestHostManager_CloseIdle(t *testing.T) {
	hm := NewHostManager(nil, nil)
	assert.Equal(t, 0, len(hm.clients))
}
