package tasks

func init() {
	Register(&UnderstandSessionLifecycle{})
	Register(&UnderstandAPIEndpoint{})
	Register(&UnderstandGateway{})
}

// UnderstandSessionLifecycle asks Claude to explain session flow.
type UnderstandSessionLifecycle struct{}

func (t *UnderstandSessionLifecycle) Name() string     { return "understand-session-lifecycle" }
func (t *UnderstandSessionLifecycle) Category() string { return "understanding" }
func (t *UnderstandSessionLifecycle) Prompt() string {
	return "Explain how a session is created, processed, and stored in verve-backend. Trace the code path from the API handler through to the database."
}
func (t *UnderstandSessionLifecycle) Validate(output string) error { return nil }

// UnderstandAPIEndpoint asks Claude to trace an API endpoint.
type UnderstandAPIEndpoint struct{}

func (t *UnderstandAPIEndpoint) Name() string     { return "understand-api-endpoint" }
func (t *UnderstandAPIEndpoint) Category() string { return "understanding" }
func (t *UnderstandAPIEndpoint) Prompt() string {
	return "What does the gateway_api/session_create endpoint do? Trace the request flow through the handler."
}
func (t *UnderstandAPIEndpoint) Validate(output string) error { return nil }

// UnderstandGateway asks Claude to explain gateway communication.
type UnderstandGateway struct{}

func (t *UnderstandGateway) Name() string     { return "understand-gateway" }
func (t *UnderstandGateway) Category() string { return "understanding" }
func (t *UnderstandGateway) Prompt() string {
	return "How does verve-gateway communicate with verve-backend? What protocol and data format are used?"
}
func (t *UnderstandGateway) Validate(output string) error { return nil }
