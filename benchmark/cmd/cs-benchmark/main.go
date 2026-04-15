package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"claude-squad/benchmark/harness"
	"claude-squad/benchmark/tasks"

	// Import task packages to register them
	_ "claude-squad/benchmark/tasks"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Parse flags
	var (
		workdir      string
		taskFilter   string
		jsonOutput   bool
		mcpOnly      bool
		noMCPOnly    bool
		compareAll   bool // Run all three: no-mcp, ctags, tree-sitter
		treesitter   bool // Use tree-sitter instead of ctags
		verbose      bool
		timeout      time.Duration
	)

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case arg == "--workdir" && i+1 < len(os.Args):
			i++
			workdir = os.Args[i]
		case arg == "--tasks" && i+1 < len(os.Args):
			i++
			taskFilter = os.Args[i]
		case arg == "--json":
			jsonOutput = true
		case arg == "--mcp-only":
			mcpOnly = true
		case arg == "--no-mcp-only":
			noMCPOnly = true
		case arg == "--compare-all":
			compareAll = true
		case arg == "--treesitter":
			treesitter = true
		case arg == "--verbose":
			verbose = true
		case arg == "--timeout" && i+1 < len(os.Args):
			i++
			var err error
			timeout, err = time.ParseDuration(os.Args[i])
			if err != nil {
				return fmt.Errorf("invalid timeout: %w", err)
			}
		case arg == "--help" || arg == "-h":
			printUsage()
			return nil
		}
	}

	if workdir == "" {
		return fmt.Errorf("--workdir is required")
	}

	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nInterrupted, cleaning up...")
		cancel()
	}()

	// Get tasks to run
	var taskList []tasks.Task
	if taskFilter == "" {
		taskList = tasks.GetAll()
	} else {
		categories := strings.Split(taskFilter, ",")
		for _, cat := range categories {
			taskList = append(taskList, tasks.GetByCategory(strings.TrimSpace(cat))...)
		}
	}

	if len(taskList) == 0 {
		return fmt.Errorf("no tasks to run")
	}

	if !jsonOutput {
		fmt.Printf("Running %d tasks in %s\n", len(taskList), workdir)
	}

	// Determine which indexer type to use
	indexerType := harness.IndexerCtags
	if treesitter {
		indexerType = harness.IndexerTreeSitter
	}

	// Start MCP servers (if needed)
	var mcpConfig string
	var ctagsConfig, treesitterConfig string
	var ctagsServer, treesitterServer *harness.MCPServer

	if compareAll {
		// Start both indexers for comparison
		if !jsonOutput {
			fmt.Println("Starting ctags MCP server...")
		}
		var err error
		ctagsServer, err = harness.StartMCPServerWithType(workdir, harness.IndexerCtags)
		if err != nil {
			return fmt.Errorf("failed to start ctags MCP server: %w", err)
		}
		defer ctagsServer.Stop()

		if !jsonOutput {
			fmt.Print("Building ctags index...")
		}
		ctagsCount := ctagsServer.WaitForIndex(30 * time.Second)
		ctagsConfig = ctagsServer.Config()
		if !jsonOutput {
			fmt.Printf(" %d symbols\n", ctagsCount)
		}

		if !jsonOutput {
			fmt.Println("Starting tree-sitter MCP server...")
		}
		treesitterServer, err = harness.StartMCPServerWithType(workdir, harness.IndexerTreeSitter)
		if err != nil {
			return fmt.Errorf("failed to start tree-sitter MCP server: %w", err)
		}
		defer treesitterServer.Stop()

		if !jsonOutput {
			fmt.Print("Building tree-sitter index...")
		}
		tsCount := treesitterServer.WaitForIndex(30 * time.Second)
		treesitterConfig = treesitterServer.Config()
		if !jsonOutput {
			fmt.Printf(" %d symbols\n", tsCount)
		}
	} else if !noMCPOnly {
		if !jsonOutput {
			fmt.Printf("Starting MCP index server (%s)...\n", indexerType)
		}
		mcpServer, err := harness.StartMCPServerWithType(workdir, indexerType)
		if err != nil {
			return fmt.Errorf("failed to start MCP server: %w", err)
		}
		defer mcpServer.Stop()

		// Wait for indexer to build (up to 30 seconds)
		if !jsonOutput {
			fmt.Print("Building symbol index...")
		}
		symbolCount := mcpServer.WaitForIndex(30 * time.Second)
		mcpConfig = mcpServer.Config()
		if !jsonOutput {
			fmt.Printf(" %d symbols indexed\n", symbolCount)
			fmt.Printf("MCP server running on port %d\n", mcpServer.Port())
		}
	}

	// Run tasks
	runner := harness.NewRunner(harness.RunConfig{
		Workdir: workdir,
		Timeout: timeout,
		Verbose: verbose,
	})

	for i, task := range taskList {
		if ctx.Err() != nil {
			break
		}

		if !jsonOutput {
			fmt.Printf("[%d/%d] Running: %s\n", i+1, len(taskList), task.Name())
		}

		// Run without MCP (baseline)
		if !mcpOnly {
			if !jsonOutput && verbose {
				fmt.Printf("  Without MCP...\n")
			}
			cfg := runner.Config
			cfg.MCPConfig = ""
			result := harness.RunTask(ctx, task, cfg)
			result.Metrics.IndexerType = "none"
			runner.Results = append(runner.Results, result)
			if !jsonOutput {
				if result.Error != nil {
					fmt.Printf("  [NO MCP] FAIL: %v\n", result.Error)
				} else {
					fmt.Printf("  [NO MCP] %d input tokens, %d tool calls\n", result.Metrics.InputTokens, len(result.Metrics.ToolCalls))
				}
			}
		}

		if compareAll {
			// Run with ctags
			if !jsonOutput && verbose {
				fmt.Printf("  With ctags MCP...\n")
			}
			cfg := runner.Config
			cfg.MCPConfig = ctagsConfig
			result := harness.RunTask(ctx, task, cfg)
			result.Metrics.IndexerType = "ctags"
			runner.Results = append(runner.Results, result)
			if !jsonOutput {
				if result.Error != nil {
					fmt.Printf("  [CTAGS] FAIL: %v\n", result.Error)
				} else {
					fmt.Printf("  [CTAGS] %d input tokens, %d tool calls, %d MCP calls\n", result.Metrics.InputTokens, len(result.Metrics.ToolCalls), result.Metrics.MCPToolCount)
				}
			}

			// Run with tree-sitter
			if !jsonOutput && verbose {
				fmt.Printf("  With tree-sitter MCP...\n")
			}
			cfg = runner.Config
			cfg.MCPConfig = treesitterConfig
			result = harness.RunTask(ctx, task, cfg)
			result.Metrics.IndexerType = "treesitter"
			runner.Results = append(runner.Results, result)
			if !jsonOutput {
				if result.Error != nil {
					fmt.Printf("  [TREESITTER] FAIL: %v\n", result.Error)
				} else {
					fmt.Printf("  [TREESITTER] %d input tokens, %d tool calls, %d MCP calls\n", result.Metrics.InputTokens, len(result.Metrics.ToolCalls), result.Metrics.MCPToolCount)
				}
			}
		} else if !noMCPOnly && mcpConfig != "" {
			// Run with single MCP
			if !jsonOutput && verbose {
				fmt.Printf("  With MCP...\n")
			}
			cfg := runner.Config
			cfg.MCPConfig = mcpConfig
			result := harness.RunTask(ctx, task, cfg)
			result.Metrics.IndexerType = string(indexerType)
			runner.Results = append(runner.Results, result)
			if !jsonOutput {
				if result.Error != nil {
					fmt.Printf("  [W/ MCP] FAIL: %v\n", result.Error)
				} else {
					fmt.Printf("  [W/ MCP] %d input tokens, %d tool calls, %d MCP calls\n", result.Metrics.InputTokens, len(result.Metrics.ToolCalls), result.Metrics.MCPToolCount)
				}
			}
		}
	}

	// Generate report
	report := harness.GenerateReport(workdir, runner.Results)

	if jsonOutput {
		report.WriteJSON(os.Stdout)
	} else {
		fmt.Println()
		report.WriteTerminal(os.Stdout)
	}

	return nil
}

func printUsage() {
	fmt.Println(`cs-benchmark - MCP Index Server Benchmark Tool

Usage:
  cs-benchmark --workdir <path> [options]

Options:
  --workdir <path>    Target codebase to run tasks against (required)
  --tasks <list>      Comma-separated categories: symbol,symbol-indexed,symbol-treesitter,understanding,edit,crossfile
  --json              Output JSON instead of terminal tables
  --mcp-only          Only run MCP-enabled variant
  --no-mcp-only       Only run MCP-disabled variant
  --compare-all       Run all three variants: no-mcp, ctags, tree-sitter
  --treesitter        Use tree-sitter indexer instead of ctags
  --verbose           Show detailed output
  --timeout <dur>     Per-task timeout (default: 5m)
  --help, -h          Show this help

Examples:
  cs-benchmark --workdir ~/code/myproject
  cs-benchmark --workdir ~/code/myproject --tasks symbol,understanding
  cs-benchmark --workdir ~/code/myproject --compare-all
  cs-benchmark --workdir ~/code/myproject --json > report.json`)
}
