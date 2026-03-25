package pty

// Subscriber receives live terminal output via a channel.
type Subscriber struct {
	Ch chan []byte
}

// StreamableSession is a terminal session that can be streamed over WebSocket.
type StreamableSession interface {
	Write(p []byte) (int, error)
	Subscribe() *Subscriber
	Unsubscribe(sub *Subscriber)
	Closed() bool
	GetSnapshot() []byte
}

// SessionRegistry looks up streamable sessions by ID.
type SessionRegistry interface {
	Get(id string) StreamableSession
}

// CompositeRegistry tries multiple registries in order.
type CompositeRegistry struct {
	registries []SessionRegistry
}

func NewCompositeRegistry(registries ...SessionRegistry) *CompositeRegistry {
	return &CompositeRegistry{registries: registries}
}

func (c *CompositeRegistry) Get(id string) StreamableSession {
	for _, r := range c.registries {
		if sess := r.Get(id); sess != nil {
			return sess
		}
	}
	return nil
}
