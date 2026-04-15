package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Report holds aggregated benchmark results.
type Report struct {
	Workdir   string                 `json:"workdir"`
	Timestamp time.Time              `json:"timestamp"`
	Summary   ReportSummary          `json:"summary"`
	ByTask    map[string]*TaskReport `json:"tasks"`
}

// ReportSummary holds aggregate metrics.
type ReportSummary struct {
	NoMCP      AggregateMetrics `json:"no_mcp"`
	WithMCP    AggregateMetrics `json:"with_mcp"`
	Ctags      AggregateMetrics `json:"ctags,omitempty"`
	TreeSitter AggregateMetrics `json:"treesitter,omitempty"`
	ChangePct  ChangeMetrics    `json:"change_pct"`
}

// AggregateMetrics holds summed metrics.
type AggregateMetrics struct {
	InputTokens  int           `json:"input_tokens"`
	OutputTokens int           `json:"output_tokens"`
	ToolCalls    int           `json:"tool_calls"`
	ReadCalls    int           `json:"read_calls"`
	GrepCalls    int           `json:"grep_calls"`
	MCPCalls     int           `json:"mcp_calls"`
	TotalTime    time.Duration `json:"total_time"`
	TasksPassed  int           `json:"tasks_passed"`
	TasksTotal   int           `json:"tasks_total"`
}

// ChangeMetrics holds percentage changes.
type ChangeMetrics struct {
	InputTokens  float64 `json:"input_tokens"`
	OutputTokens float64 `json:"output_tokens"`
	ToolCalls    float64 `json:"tool_calls"`
	TotalTime    float64 `json:"total_time"`
}

// TaskReport holds per-task comparison.
type TaskReport struct {
	Name       string         `json:"name"`
	Category   string         `json:"category"`
	NoMCP      *TaskMetrics   `json:"no_mcp,omitempty"`
	WithMCP    *TaskMetrics   `json:"with_mcp,omitempty"`
	Ctags      *TaskMetrics   `json:"ctags,omitempty"`
	TreeSitter *TaskMetrics   `json:"treesitter,omitempty"`
	ChangePct  *ChangeMetrics `json:"change_pct,omitempty"`
}

// GenerateReport creates a report from run results.
func GenerateReport(workdir string, results []*RunResult) *Report {
	report := &Report{
		Workdir:   workdir,
		Timestamp: time.Now(),
		ByTask:    make(map[string]*TaskReport),
	}

	// Group results by task
	for _, r := range results {
		taskName := r.Task.Name()
		if _, ok := report.ByTask[taskName]; !ok {
			report.ByTask[taskName] = &TaskReport{
				Name:     taskName,
				Category: r.Task.Category(),
			}
		}

		tr := report.ByTask[taskName]
		indexerType := r.Metrics.IndexerType
		if indexerType == "" {
			// Backwards compatibility: infer from MCPEnabled
			if r.MCPEnabled {
				indexerType = "ctags"
			} else {
				indexerType = "none"
			}
		}

		// Add metrics to appropriate bucket based on indexer type
		switch indexerType {
		case "none":
			tr.NoMCP = r.Metrics
			addToAggregate(&report.Summary.NoMCP, r.Metrics)
		case "ctags":
			tr.Ctags = r.Metrics
			tr.WithMCP = r.Metrics // For backward compat
			addToAggregate(&report.Summary.Ctags, r.Metrics)
			addToAggregate(&report.Summary.WithMCP, r.Metrics) // For backward compat
		case "treesitter":
			tr.TreeSitter = r.Metrics
			tr.WithMCP = r.Metrics // For backward compat in two-column mode
			addToAggregate(&report.Summary.TreeSitter, r.Metrics)
			addToAggregate(&report.Summary.WithMCP, r.Metrics) // For backward compat
		}
	}

	// Calculate percentage changes (comparing NoMCP to best MCP variant)
	bestMCP := report.Summary.WithMCP
	if report.Summary.TreeSitter.TasksTotal > 0 {
		bestMCP = report.Summary.TreeSitter
	}
	report.Summary.ChangePct = calcChange(report.Summary.NoMCP, bestMCP)

	// Calculate per-task changes
	for _, tr := range report.ByTask {
		if tr.NoMCP != nil && tr.WithMCP != nil {
			change := ChangeMetrics{
				InputTokens:  pctChange(tr.NoMCP.InputTokens, tr.WithMCP.InputTokens),
				OutputTokens: pctChange(tr.NoMCP.OutputTokens, tr.WithMCP.OutputTokens),
				ToolCalls:    pctChange(len(tr.NoMCP.ToolCalls), len(tr.WithMCP.ToolCalls)),
				TotalTime:    pctChange(int(tr.NoMCP.WallTime.Milliseconds()), int(tr.WithMCP.WallTime.Milliseconds())),
			}
			tr.ChangePct = &change
		}
	}

	return report
}

func addToAggregate(agg *AggregateMetrics, m *TaskMetrics) {
	agg.TasksTotal++
	if m.Success {
		agg.TasksPassed++
	}
	agg.InputTokens += m.InputTokens
	agg.OutputTokens += m.OutputTokens
	agg.ToolCalls += len(m.ToolCalls)
	agg.ReadCalls += m.ReadCount
	agg.GrepCalls += m.GrepCount
	agg.MCPCalls += m.MCPToolCount
	agg.TotalTime += m.WallTime
}

func pctChange(old, new int) float64 {
	if old == 0 {
		return 0
	}
	return float64(new-old) / float64(old) * 100
}

func calcChange(old, new AggregateMetrics) ChangeMetrics {
	return ChangeMetrics{
		InputTokens:  pctChange(old.InputTokens, new.InputTokens),
		OutputTokens: pctChange(old.OutputTokens, new.OutputTokens),
		ToolCalls:    pctChange(old.ToolCalls, new.ToolCalls),
		TotalTime:    pctChange(int(old.TotalTime.Milliseconds()), int(new.TotalTime.Milliseconds())),
	}
}

// WriteJSON writes the report as JSON.
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteTerminal writes a formatted terminal report.
func (r *Report) WriteTerminal(w io.Writer) {
	fmt.Fprintf(w, "MCP Index Benchmark Report\n")
	fmt.Fprintf(w, "==========================\n")
	fmt.Fprintf(w, "Workdir: %s\n", r.Workdir)
	fmt.Fprintf(w, "Date: %s\n\n", r.Timestamp.Format("2006-01-02 15:04:05"))

	// Check if we have all three variants
	hasThreeWay := r.Summary.TreeSitter.TasksTotal > 0 && r.Summary.Ctags.TasksTotal > 0

	fmt.Fprintf(w, "SUMMARY\n")
	if hasThreeWay {
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 90))
		fmt.Fprintf(w, "%-20s %12s %12s %12s %12s\n", "", "No Index", "Ctags", "TreeSitter", "TS vs None")
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 90))
		tsChange := calcChange(r.Summary.NoMCP, r.Summary.TreeSitter)
		fmt.Fprintf(w, "%-20s %12d %12d %12d %+11.0f%%\n", "Input Tokens", r.Summary.NoMCP.InputTokens, r.Summary.Ctags.InputTokens, r.Summary.TreeSitter.InputTokens, tsChange.InputTokens)
		fmt.Fprintf(w, "%-20s %12d %12d %12d %+11.0f%%\n", "Output Tokens", r.Summary.NoMCP.OutputTokens, r.Summary.Ctags.OutputTokens, r.Summary.TreeSitter.OutputTokens, tsChange.OutputTokens)
		fmt.Fprintf(w, "%-20s %12d %12d %12d %+11.0f%%\n", "Tool Calls", r.Summary.NoMCP.ToolCalls, r.Summary.Ctags.ToolCalls, r.Summary.TreeSitter.ToolCalls, tsChange.ToolCalls)
		fmt.Fprintf(w, "%-20s %12d %12d %12d\n", "Read Calls", r.Summary.NoMCP.ReadCalls, r.Summary.Ctags.ReadCalls, r.Summary.TreeSitter.ReadCalls)
		fmt.Fprintf(w, "%-20s %12d %12d %12d\n", "Grep Calls", r.Summary.NoMCP.GrepCalls, r.Summary.Ctags.GrepCalls, r.Summary.TreeSitter.GrepCalls)
		fmt.Fprintf(w, "%-20s %12s %12d %12d\n", "MCP Calls", "-", r.Summary.Ctags.MCPCalls, r.Summary.TreeSitter.MCPCalls)
		fmt.Fprintf(w, "%-20s %12s %12s %12s %+11.0f%%\n", "Total Time", formatDuration(r.Summary.NoMCP.TotalTime), formatDuration(r.Summary.Ctags.TotalTime), formatDuration(r.Summary.TreeSitter.TotalTime), tsChange.TotalTime)
		fmt.Fprintf(w, "%-20s %9d/%d %9d/%d %9d/%d\n", "Tasks Passed", r.Summary.NoMCP.TasksPassed, r.Summary.NoMCP.TasksTotal, r.Summary.Ctags.TasksPassed, r.Summary.Ctags.TasksTotal, r.Summary.TreeSitter.TasksPassed, r.Summary.TreeSitter.TasksTotal)
		fmt.Fprintf(w, "%s\n\n", strings.Repeat("-", 90))
	} else {
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 70))
		fmt.Fprintf(w, "%-25s %12s %12s %10s\n", "", "No MCP", "With MCP", "Change")
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 70))
		fmt.Fprintf(w, "%-25s %12d %12d %+9.0f%%\n", "Input Tokens", r.Summary.NoMCP.InputTokens, r.Summary.WithMCP.InputTokens, r.Summary.ChangePct.InputTokens)
		fmt.Fprintf(w, "%-25s %12d %12d %+9.0f%%\n", "Output Tokens", r.Summary.NoMCP.OutputTokens, r.Summary.WithMCP.OutputTokens, r.Summary.ChangePct.OutputTokens)
		fmt.Fprintf(w, "%-25s %12d %12d %+9.0f%%\n", "Tool Calls", r.Summary.NoMCP.ToolCalls, r.Summary.WithMCP.ToolCalls, r.Summary.ChangePct.ToolCalls)
		fmt.Fprintf(w, "%-25s %12d %12d\n", "Read Calls", r.Summary.NoMCP.ReadCalls, r.Summary.WithMCP.ReadCalls)
		fmt.Fprintf(w, "%-25s %12d %12d\n", "Grep Calls", r.Summary.NoMCP.GrepCalls, r.Summary.WithMCP.GrepCalls)
		fmt.Fprintf(w, "%-25s %12s %12d\n", "MCP Calls", "-", r.Summary.WithMCP.MCPCalls)
		fmt.Fprintf(w, "%-25s %12s %12s %+9.0f%%\n", "Total Time", formatDuration(r.Summary.NoMCP.TotalTime), formatDuration(r.Summary.WithMCP.TotalTime), r.Summary.ChangePct.TotalTime)
		fmt.Fprintf(w, "%-25s %9d/%d %9d/%d\n", "Tasks Passed", r.Summary.NoMCP.TasksPassed, r.Summary.NoMCP.TasksTotal, r.Summary.WithMCP.TasksPassed, r.Summary.WithMCP.TasksTotal)
		fmt.Fprintf(w, "%s\n\n", strings.Repeat("-", 70))
	}

	fmt.Fprintf(w, "BY TASK\n")
	if hasThreeWay {
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 100))
		fmt.Fprintf(w, "%-25s %10s %10s %10s %10s %10s\n", "Task", "No Index", "Ctags", "TreeSitter", "TS Change", "Status")
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 100))
		for _, tr := range r.ByTask {
			noTokens, ctagsTokens, tsTokens := 0, 0, 0
			status := "PASS"
			change := 0.0
			if tr.NoMCP != nil {
				noTokens = tr.NoMCP.InputTokens
				if !tr.NoMCP.Success {
					status = "FAIL"
				}
			}
			if tr.Ctags != nil {
				ctagsTokens = tr.Ctags.InputTokens
				if !tr.Ctags.Success {
					status = "FAIL"
				}
			}
			if tr.TreeSitter != nil {
				tsTokens = tr.TreeSitter.InputTokens
				if !tr.TreeSitter.Success {
					status = "FAIL"
				}
			}
			if noTokens > 0 {
				change = pctChange(noTokens, tsTokens)
			}
			fmt.Fprintf(w, "%-25s %10d %10d %10d %+9.0f%% %10s\n", tr.Name, noTokens, ctagsTokens, tsTokens, change, status)
		}
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 100))
	} else {
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))
		fmt.Fprintf(w, "%-30s %10s %10s %10s %10s\n", "Task", "No MCP", "With MCP", "Change", "Status")
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))
		for _, tr := range r.ByTask {
			noMCPTokens := 0
			withMCPTokens := 0
			status := "?"
			change := 0.0
			if tr.NoMCP != nil {
				noMCPTokens = tr.NoMCP.InputTokens
			}
			if tr.WithMCP != nil {
				withMCPTokens = tr.WithMCP.InputTokens
			}
			if tr.ChangePct != nil {
				change = tr.ChangePct.InputTokens
			}
			if (tr.NoMCP == nil || tr.NoMCP.Success) && (tr.WithMCP == nil || tr.WithMCP.Success) {
				status = "PASS"
			} else {
				status = "FAIL"
			}
			fmt.Fprintf(w, "%-30s %10d %10d %+9.0f%% %10s\n", tr.Name, noMCPTokens, withMCPTokens, change, status)
		}
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}
