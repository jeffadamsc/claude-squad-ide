package tasks

func init() {
	Register(&CrossfileNewEndpoint{})
	Register(&CrossfileRefactor{})
}

// CrossfileNewEndpoint asks Claude to add a new endpoint.
type CrossfileNewEndpoint struct{}

func (t *CrossfileNewEndpoint) Name() string     { return "crossfile-new-endpoint" }
func (t *CrossfileNewEndpoint) Category() string { return "crossfile" }
func (t *CrossfileNewEndpoint) Prompt() string {
	return "Add a new health check endpoint to verve-backend following the pattern of existing gateway_api endpoints. The endpoint should be at /health and return a simple JSON response with status: ok."
}
func (t *CrossfileNewEndpoint) Validate(output string) error { return nil }

// CrossfileRefactor asks Claude to make code consistent across files.
type CrossfileRefactor struct{}

func (t *CrossfileRefactor) Name() string     { return "crossfile-refactor" }
func (t *CrossfileRefactor) Category() string { return "crossfile" }
func (t *CrossfileRefactor) Prompt() string {
	return "Look at how error handling is done in verve-backend/pkg/api. Find an inconsistency in how errors are returned and make it consistent with the predominant pattern."
}
func (t *CrossfileRefactor) Validate(output string) error { return nil }
