package app

import (
	"claude-squad/session"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPollAllStatuses_AutoPauseIdle(t *testing.T) {
	api := newTestAPI(t)
	api.idleTimeout = 1 * time.Millisecond // tiny timeout for testing

	_, err := api.CreateSession(CreateOptions{
		Title:   "idle-test",
		Path:    t.TempDir(),
		Program: "echo hello",
		InPlace: true,
	})
	require.NoError(t, err)

	inst := api.instances["idle-test"]
	require.NotNil(t, inst)

	// Simulate: session is idle for longer than timeout
	inst.MarkIdle()
	inst.IdleSince = time.Now().Add(-1 * time.Hour)

	statuses, err := api.PollAllStatuses()
	require.NoError(t, err)
	require.Len(t, statuses, 1)

	assert.Equal(t, "paused", statuses[0].Status)
	assert.True(t, statuses[0].AutoPaused)
}

func TestPollAllStatuses_SkipRecentlyViewed(t *testing.T) {
	api := newTestAPI(t)
	api.idleTimeout = 1 * time.Millisecond

	_, err := api.CreateSession(CreateOptions{
		Title:   "viewed-test",
		Path:    t.TempDir(),
		Program: "echo hello",
		InPlace: true,
	})
	require.NoError(t, err)

	inst := api.instances["viewed-test"]

	inst.MarkIdle()
	inst.IdleSince = time.Now().Add(-1 * time.Hour)
	inst.TouchLastViewed() // viewed just now

	statuses, err := api.PollAllStatuses()
	require.NoError(t, err)
	require.Len(t, statuses, 1)

	assert.NotEqual(t, "paused", statuses[0].Status)
}

func TestPollAllStatuses_DisabledTimeout(t *testing.T) {
	api := newTestAPI(t)
	api.idleTimeout = 0 // disabled

	_, err := api.CreateSession(CreateOptions{
		Title:   "no-timeout",
		Path:    t.TempDir(),
		Program: "echo hello",
		InPlace: true,
	})
	require.NoError(t, err)

	inst := api.instances["no-timeout"]
	inst.MarkIdle()
	inst.IdleSince = time.Now().Add(-24 * time.Hour)

	statuses, err := api.PollAllStatuses()
	require.NoError(t, err)
	require.Len(t, statuses, 1)

	assert.NotEqual(t, "paused", statuses[0].Status)
}

// Ensure the Instance fields/methods used by these tests are accessible.
var _ = (*session.Instance)(nil)
