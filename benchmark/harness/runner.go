package harness

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"claude-squad/benchmark/tasks"
)

// RunConfig holds configuration for a benchmark run.
type RunConfig struct {
	Workdir   string
	MCPConfig string // Empty = no MCP
	Timeout   time.Duration
	Verbose   bool
}

// RunResult holds the result of running a single task.
type RunResult struct {
	Task       tasks.Task
	MCPEnabled bool
	Metrics    *TaskMetrics
	Output     string
	Error      error
}

// RunTask runs a single task with the given configuration.
func RunTask(ctx context.Context, task tasks.Task, cfg RunConfig) *RunResult {
	result := &RunResult{
		Task:       task,
		MCPEnabled: cfg.MCPConfig != "",
	}

	// Build command arguments
	// Note: --verbose is required when using --output-format=stream-json with --print
	args := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"-p", task.Prompt(),
	}

	if cfg.MCPConfig != "" {
		args = append(args, "--mcp-config", cfg.MCPConfig)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = cfg.Workdir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	wallTime := time.Since(startTime)

	result.Output = stdout.String()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = fmt.Errorf("timeout after %v", cfg.Timeout)
		} else {
			result.Error = fmt.Errorf("claude exited with error: %w\nstderr: %s", err, stderr.String())
		}
		result.Metrics = &TaskMetrics{
			TaskName:   task.Name(),
			MCPEnabled: cfg.MCPConfig != "",
			WallTime:   wallTime,
			Success:    false,
			Error:      result.Error.Error(),
		}
		return result
	}

	// Parse metrics from output
	metrics, err := ParseMetrics(result.Output)
	if err != nil {
		result.Error = fmt.Errorf("failed to parse metrics: %w", err)
		result.Metrics = &TaskMetrics{
			TaskName:   task.Name(),
			MCPEnabled: cfg.MCPConfig != "",
			WallTime:   wallTime,
			Success:    false,
			Error:      result.Error.Error(),
		}
		return result
	}

	metrics.TaskName = task.Name()
	metrics.MCPEnabled = cfg.MCPConfig != ""
	metrics.WallTime = wallTime
	result.Metrics = metrics

	// Run validation if provided
	if err := task.Validate(result.Output); err != nil {
		result.Metrics.Success = false
		result.Metrics.Error = fmt.Sprintf("validation failed: %v", err)
	}

	return result
}

// Runner manages running multiple tasks.
type Runner struct {
	Config  RunConfig
	Results []*RunResult
}

// NewRunner creates a new benchmark runner.
func NewRunner(cfg RunConfig) *Runner {
	return &Runner{Config: cfg}
}

// Run executes all provided tasks with and without MCP.
func (r *Runner) Run(ctx context.Context, taskList []tasks.Task, mcpConfig string, mcpOnly, noMCPOnly bool) {
	for _, task := range taskList {
		// Run without MCP
		if !mcpOnly {
			cfg := r.Config
			cfg.MCPConfig = ""
			result := RunTask(ctx, task, cfg)
			r.Results = append(r.Results, result)
		}

		// Run with MCP
		if !noMCPOnly {
			cfg := r.Config
			cfg.MCPConfig = mcpConfig
			result := RunTask(ctx, task, cfg)
			r.Results = append(r.Results, result)
		}
	}
}
