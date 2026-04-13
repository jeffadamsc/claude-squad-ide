package harness

import (
	"bufio"
	"encoding/json"
	"strings"
	"time"
)

// TaskMetrics holds the metrics collected from a single task run.
type TaskMetrics struct {
	TaskName   string
	MCPEnabled bool

	// Token usage
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CacheReads   int
	CacheWrites  int

	// Tool usage
	ToolCalls    []ToolCall
	ReadCount    int
	GrepCount    int
	EditCount    int
	MCPToolCount int

	// Timing
	WallTime time.Duration

	// Outcome
	Success bool
	Error   string
}

// ToolCall represents a single tool invocation.
type ToolCall struct {
	Name         string
	InputTokens  int
	OutputTokens int
}

// streamMessage represents a line from Claude's stream-json output.
type streamMessage struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype,omitempty"`
	Message *struct {
		Content []struct {
			Type string `json:"type"`
			Name string `json:"name,omitempty"`
		} `json:"content,omitempty"`
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage,omitempty"`
	} `json:"message,omitempty"`

	// Result fields
	IsError    bool   `json:"is_error,omitempty"`
	ErrorMsg   string `json:"error,omitempty"`
	DurationMs int    `json:"duration_ms,omitempty"`
	Usage      *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
}

// ParseMetrics parses Claude's stream-json output into TaskMetrics.
func ParseMetrics(output string) (*TaskMetrics, error) {
	metrics := &TaskMetrics{
		Success: true,
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg streamMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // Skip unparseable lines
		}

		switch msg.Type {
		case "assistant":
			if msg.Message != nil {
				// Extract tool calls
				for _, content := range msg.Message.Content {
					if content.Type == "tool_use" {
						tc := ToolCall{Name: content.Name}
						metrics.ToolCalls = append(metrics.ToolCalls, tc)

						// Count by tool type
						switch content.Name {
						case "Read":
							metrics.ReadCount++
						case "Grep":
							metrics.GrepCount++
						case "Edit", "Write":
							metrics.EditCount++
						case "lookup_symbol", "list_files", "get_file_outline", "read_lines", "search_symbols":
							metrics.MCPToolCount++
						}
					}
				}
			}

		case "result":
			if msg.Usage != nil {
				metrics.InputTokens = msg.Usage.InputTokens
				metrics.OutputTokens = msg.Usage.OutputTokens
				metrics.TotalTokens = msg.Usage.InputTokens + msg.Usage.OutputTokens
			}
			metrics.WallTime = time.Duration(msg.DurationMs) * time.Millisecond

			if msg.IsError {
				metrics.Success = false
				metrics.Error = msg.ErrorMsg
			}
		}
	}

	return metrics, scanner.Err()
}
