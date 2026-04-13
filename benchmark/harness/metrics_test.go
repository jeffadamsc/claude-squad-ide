package harness

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMetrics(t *testing.T) {
	// Sample stream-json output from Claude
	output := `{"type":"system","subtype":"init","apiKeySource":"keychain"}
{"type":"assistant","message":{"id":"msg_01","role":"assistant","content":[{"type":"tool_use","id":"tu_1","name":"Read","input":{"file_path":"/test.go"}}],"usage":{"input_tokens":100,"output_tokens":50}}}
{"type":"assistant","message":{"id":"msg_02","role":"assistant","content":[{"type":"tool_use","id":"tu_2","name":"Grep","input":{"pattern":"func"}}],"usage":{"input_tokens":150,"output_tokens":75}}}
{"type":"result","subtype":"success","cost_usd":0.01,"is_error":false,"duration_ms":5000,"duration_api_ms":4500,"input_tokens":250,"output_tokens":125}`

	metrics, err := ParseMetrics(output)
	require.NoError(t, err)

	assert.Equal(t, 250, metrics.InputTokens)
	assert.Equal(t, 125, metrics.OutputTokens)
	assert.Equal(t, 2, len(metrics.ToolCalls))
	assert.Equal(t, "Read", metrics.ToolCalls[0].Name)
	assert.Equal(t, "Grep", metrics.ToolCalls[1].Name)
	assert.Equal(t, 1, metrics.ReadCount)
	assert.Equal(t, 1, metrics.GrepCount)
	assert.True(t, metrics.Success)
}

func TestParseMetrics_Error(t *testing.T) {
	output := `{"type":"result","subtype":"error","is_error":true,"error":"API error"}`

	metrics, err := ParseMetrics(output)
	require.NoError(t, err)
	assert.False(t, metrics.Success)
	assert.Equal(t, "API error", metrics.Error)
}
