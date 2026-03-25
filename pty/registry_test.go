package pty

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockStreamableSession struct {
	data    []byte
	closed  bool
	written []byte
}

func (m *mockStreamableSession) Write(p []byte) (int, error) {
	m.written = append(m.written, p...)
	return len(p), nil
}
func (m *mockStreamableSession) Subscribe() *Subscriber   { return &Subscriber{Ch: make(chan []byte, 1)} }
func (m *mockStreamableSession) Unsubscribe(sub *Subscriber) {}
func (m *mockStreamableSession) Closed() bool              { return m.closed }
func (m *mockStreamableSession) GetSnapshot() []byte       { return m.data }

type mockRegistry struct {
	sessions map[string]StreamableSession
}

func (r *mockRegistry) Get(id string) StreamableSession { return r.sessions[id] }

func TestCompositeRegistry_PrimaryFirst(t *testing.T) {
	primary := &mockRegistry{sessions: map[string]StreamableSession{
		"s1": &mockStreamableSession{data: []byte("primary")},
	}}
	secondary := &mockRegistry{sessions: map[string]StreamableSession{
		"s1": &mockStreamableSession{data: []byte("secondary")},
	}}
	composite := NewCompositeRegistry(primary, secondary)

	sess := composite.Get("s1")
	assert.NotNil(t, sess)
	assert.Equal(t, []byte("primary"), sess.GetSnapshot())
}

func TestCompositeRegistry_Fallback(t *testing.T) {
	primary := &mockRegistry{sessions: map[string]StreamableSession{}}
	secondary := &mockRegistry{sessions: map[string]StreamableSession{
		"s2": &mockStreamableSession{data: []byte("secondary")},
	}}
	composite := NewCompositeRegistry(primary, secondary)

	sess := composite.Get("s2")
	assert.NotNil(t, sess)
	assert.Equal(t, []byte("secondary"), sess.GetSnapshot())
}

func TestCompositeRegistry_NotFound(t *testing.T) {
	primary := &mockRegistry{sessions: map[string]StreamableSession{}}
	secondary := &mockRegistry{sessions: map[string]StreamableSession{}}
	composite := NewCompositeRegistry(primary, secondary)

	assert.Nil(t, composite.Get("missing"))
}
