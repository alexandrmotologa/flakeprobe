package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexandrmotologa/flakeprobe/pkg/cockpit"
	"github.com/alexandrmotologa/flakeprobe/pkg/config"
	"github.com/alexandrmotologa/flakeprobe/pkg/diff"
	"github.com/alexandrmotologa/flakeprobe/pkg/entropy"
	"github.com/alexandrmotologa/flakeprobe/pkg/mockagent"
	"github.com/alexandrmotologa/flakeprobe/pkg/recorder"
	"github.com/alexandrmotologa/flakeprobe/pkg/report"
	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
	"github.com/alexandrmotologa/flakeprobe/pkg/suggest"
)

const (
	version = "1.1.0"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "test":
		runTestCommand(os.Args[2:])
	case "diff":
		runDiffCommand(os.Args[2:])
	case "suggest":
		runSuggestCommand(os.Args[2:])
	case "sweep":
		runSweepCommand(os.Args[2:])
	case "record":
		runRecordCommand(os.Args[2:])
	case "replay":
		runReplayCommand(os.Args[2:])
	case "cockpit":
		runCockpitCommand(os.Args[2:])
	case "mock-agent":
		runMockAgentCommand(os.Args[2:])
	case "version":
		fmt.Printf("flakeprobe v%s (Go CLI & Diagnostic Engine)\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`flakeprobe - AI Agent Reliability & Non-Deterministic Regression Benchmarker

Usage:
  flakeprobe <command> [arguments]

Commands:
  test        Execute N parallel runs against an agent target and compute path entropy & FRI
  diff        Compare two execution trajectories side-by-side to highlight divergence points
  suggest     Generate automated prompt disambiguation directives from flakiness analysis
  sweep       Execute parameter matrix sweep (temperatures, prompts) from flakeprobe.yaml
  record      Execute agent target and synthesize a deterministic replay fixture (.flakerec.json)
  replay      Host a deterministic mock server from a recorded fixture for fast CI regression testing
  cockpit     Launch the embedded web visualization dashboard on port 8484
  mock-agent  Launch the synthetic test agent HTTP server with controllable flakiness archetypes
  version     Print flakeprobe version

Run 'flakeprobe <command> --help' for details on each subcommand.`)
}

func runTestCommand(args []string) {
	fs := flag.NewFlagSet("test", flag.ExitOnError)

	endpoint := fs.String("endpoint", "", "OpenAI-compatible API endpoint URL")
	apiKey := fs.String("api-key", os.Getenv("OPENAI_API_KEY"), "API key for target endpoint")
	model := fs.String("model", "gpt-4o-mini", "Model identifier for endpoint")
	prompt := fs.String("prompt", "Analyze system health and query error metrics", "Prompt provided to the agent")
	systemPrompt := fs.String("system-prompt", "", "Optional system prompt")
	execCmd := fs.String("exec", "", "CLI command to spawn external agent binary")
	mcpCmd := fs.String("mcp", "", "Command to run MCP stdio server")
	mockArchetype := fs.String("mock-archetype", "", "Use built-in mock archetype: deterministic, low-jitter, bifurcated, hallucination-loop, flaky-failure")
	runs := fs.Int("runs", 10, "Number of test iterations to execute")
	concurrency := fs.Int("concurrency", 4, "Number of concurrent execution workers")
	jitterMs := fs.Int("jitter", 0, "Maximum artificial jitter delay injected between runs in milliseconds")
	maxFRI := fs.Float64("max-fri", 0.20, "Maximum acceptable Flakiness Risk Index threshold before CI failure")
	outPath := fs.String("out", "", "File path to save JSON analysis report")
	junitPath := fs.String("junit", "", "File path to save JUnit XML test report")
	summaryPath := fs.String("github-summary", "", "File path for GitHub Step Summary (defaults to GITHUB_STEP_SUMMARY env var)")
	fixtureOut := fs.String("fixture-out", "", "File path to export synthesized deterministic replay contract (.flakerec.json)")
	showSuggest := fs.Bool("suggest", false, "Print automated prompt mitigation directives if bifurcations are found")
	launchCockpit := fs.Bool("cockpit", false, "Launch interactive web cockpit server")
	cockpitPort := fs.Int("port", 8484, "Port for web cockpit dashboard")

	_ = fs.Parse(args)

	targetType := "openai"
	targetDesc := ""

	if *mockArchetype != "" {
		targetType = "mock"
		targetDesc = fmt.Sprintf("Built-in Mock Agent (archetype: %s)", *mockArchetype)
	} else if *execCmd != "" {
		targetType = "exec"
		targetDesc = fmt.Sprintf("CLI Subprocess: %s", *execCmd)
	} else if *mcpCmd != "" {
		targetType = "mcp"
		targetDesc = fmt.Sprintf("MCP Stdio: %s", *mcpCmd)
	} else if *endpoint != "" {
		targetType = "openai"
		targetDesc = fmt.Sprintf("OpenAI API (%s at %s)", *model, *endpoint)
	} else {
		targetType = "mock"
		*mockArchetype = "deterministic"
		targetDesc = "Built-in Mock Agent (archetype: deterministic)"
	}

	orchestrator := runner.NewOrchestrator()
	orchestrator.RegisterAdapter("openai", runner.NewOpenAIAdapter())
	orchestrator.RegisterAdapter("exec", runner.NewExecAdapter())
	orchestrator.RegisterAdapter("mcp", runner.NewMCPAdapter())
	orchestrator.RegisterAdapter("mock", mockagent.NewInProcessMockAdapter())

	suiteConfig := runner.SuiteConfig{
		Target: runner.TargetConfig{
			Type:          targetType,
			Endpoint:      *endpoint,
			APIKey:        *apiKey,
			Model:         *model,
			Prompt:        *prompt,
			SystemPrompt:  *systemPrompt,
			ExecCommand:   *execCmd,
			MCPCommand:    *mcpCmd,
			MockArchetype: *mockArchetype,
			MaxSteps:      10,
			TimeoutSec:    60,
			Temperature:   0.0,
		},
		TotalRuns:   *runs,
		Concurrency: *concurrency,
		JitterMs:    *jitterMs,
		MaxFRI:      *maxFRI,
	}

	fmt.Printf("[flakeprobe] Initiating %d runs against %s (concurrency: %d)...\n", *runs, targetDesc, *concurrency)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runResults, err := orchestrator.RunSuite(ctx, suiteConfig, func(p runner.StepProgress) {
		if p.Status == "step" {
			fmt.Printf("  -> [%s] Step %d: %s\n", p.RunID, p.StepIndex, p.ToolName)
		}
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error during test suite execution: %v\n", err)
		os.Exit(1)
	}

	// Statistical Analysis
	analysis := entropy.AnalyzeFlakiness(runResults, *maxFRI, nil)

	// Render Terminal Output
	report.RenderTerminal(os.Stdout, analysis, runResults, targetDesc)

	// Optional Prompt Mitigations
	if *showSuggest || analysis.IsUnstable {
		mitigations := suggest.GenerateMitigations(analysis)
		suggest.RenderTerminalSuggestions(os.Stdout, mitigations)
	}

	// Artifact Exports
	if *outPath != "" {
		if err := report.ExportJSON(*outPath, analysis, runResults, targetDesc); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to save JSON report: %v\n", err)
		} else {
			fmt.Printf("[flakeprobe] JSON report exported to %s\n", *outPath)
		}
	}

	if *junitPath != "" {
		if err := report.ExportJUnitXML(*junitPath, analysis, targetDesc); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to save JUnit XML report: %v\n", err)
		} else {
			fmt.Printf("[flakeprobe] JUnit XML report exported to %s\n", *junitPath)
		}
	}

	if *summaryPath != "" || os.Getenv("GITHUB_STEP_SUMMARY") != "" {
		if err := report.ExportGitHubSummary(*summaryPath, analysis, targetDesc); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to export GitHub Step Summary: %v\n", err)
		} else {
			fmt.Println("[flakeprobe] GitHub Step Summary updated successfully.")
		}
	}

	if *fixtureOut != "" {
		fixture, err := recorder.SynthesizeFixture(runResults, analysis, *prompt, targetType)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to synthesize replay fixture: %v\n", err)
		} else {
			if err := recorder.SaveFixture(fixture, *fixtureOut); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to save fixture: %v\n", err)
			} else {
				fmt.Printf("[flakeprobe] Deterministic replay contract saved to %s\n", *fixtureOut)
			}
		}
	}

	// Optional Cockpit Server
	if *launchCockpit {
		fullReport := &report.FullReport{
			Timestamp:  time.Now().UTC(),
			TargetDesc: targetDesc,
			Analysis:   analysis,
			Runs:       runResults,
		}
		srv := cockpit.NewServer(*cockpitPort, fullReport)
		go func() {
			fmt.Printf("[flakeprobe] Launching Cockpit UI at http://127.0.0.1:%d ...\n", *cockpitPort)
			if err := srv.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Cockpit server error: %v\n", err)
			}
		}()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		fmt.Println("[flakeprobe] Press Ctrl+C to stop cockpit server.")
		<-sigChan
		_ = srv.Stop(context.Background())
	}

	// CI Gate evaluation
	if analysis.IsUnstable {
		fmt.Printf("\n[flakeprobe] GATE FAILED: Flakiness Risk Index (%.4f) exceeded threshold (%.4f).\n", analysis.FRI, analysis.Threshold)
		os.Exit(1)
	}

	fmt.Printf("\n[flakeprobe] GATE PASSED: Agent meets stability threshold.\n")
}

func runDiffCommand(args []string) {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	reportPath := fs.String("report", "", "Path to report.json containing executed runs")
	runAID := fs.String("run1", "run-001", "First run ID to compare")
	runBID := fs.String("run2", "run-002", "Second run ID to compare")

	_ = fs.Parse(args)

	if *reportPath == "" {
		fmt.Fprintf(os.Stderr, "Error: --report <path> is required\n")
		os.Exit(1)
	}

	rep, err := report.LoadReport(*reportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load report: %v\n", err)
		os.Exit(1)
	}

	var runA, runB *runner.Run
	for i := range rep.Runs {
		if rep.Runs[i].ID == *runAID {
			runA = &rep.Runs[i]
		}
		if rep.Runs[i].ID == *runBID {
			runB = &rep.Runs[i]
		}
	}

	if runA == nil {
		fmt.Fprintf(os.Stderr, "Error: %s not found in report\n", *runAID)
		os.Exit(1)
	}
	if runB == nil {
		fmt.Fprintf(os.Stderr, "Error: %s not found in report\n", *runBID)
		os.Exit(1)
	}

	d := diff.CompareRuns(*runA, *runB)
	diff.RenderTerminalDiff(os.Stdout, d)
}

func runSuggestCommand(args []string) {
	fs := flag.NewFlagSet("suggest", flag.ExitOnError)
	reportPath := fs.String("report", "", "Path to report.json containing flakiness analysis")

	_ = fs.Parse(args)

	if *reportPath == "" {
		fmt.Fprintf(os.Stderr, "Error: --report <path> is required\n")
		os.Exit(1)
	}

	rep, err := report.LoadReport(*reportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load report: %v\n", err)
		os.Exit(1)
	}

	mitigations := suggest.GenerateMitigations(rep.Analysis)
	suggest.RenderTerminalSuggestions(os.Stdout, mitigations)
}

func runSweepCommand(args []string) {
	fs := flag.NewFlagSet("sweep", flag.ExitOnError)
	configPath := fs.String("config", "flakeprobe.yaml", "Path to configuration file")

	_ = fs.Parse(args)

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration from %s: %v\n", *configPath, err)
		os.Exit(1)
	}

	temps := cfg.Sweep.Temperatures
	if len(temps) == 0 {
		temps = []float64{0.0, 0.2, 0.5, 0.7}
	}

	orchestrator := runner.NewOrchestrator()
	orchestrator.RegisterAdapter("openai", runner.NewOpenAIAdapter())
	orchestrator.RegisterAdapter("exec", runner.NewExecAdapter())
	orchestrator.RegisterAdapter("mcp", runner.NewMCPAdapter())
	orchestrator.RegisterAdapter("mock", mockagent.NewInProcessMockAdapter())

	sweepRes := config.SweepResult{
		Entries:             make([]config.SweepEntry, 0),
		PhaseTransitionTemp: -1,
		OptimalTemperature:  temps[0],
	}

	fmt.Printf("[flakeprobe] Executing temperature sweep across %d temperature steps...\n", len(temps))

	lowestFRI := 2.0

	for _, t := range temps {
		targetCfg := cfg.Target.ToRunnerConfig()
		targetCfg.Temperature = t

		suiteCfg := runner.SuiteConfig{
			Target:      targetCfg,
			TotalRuns:   cfg.Suite.Runs,
			Concurrency: cfg.Suite.Concurrency,
			MaxFRI:      cfg.Suite.MaxFRI,
		}

		runs, err := orchestrator.RunSuite(context.Background(), suiteCfg, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Sweep step T=%.2f failed: %v\n", t, err)
			continue
		}

		analysis := entropy.AnalyzeFlakiness(runs, cfg.Suite.MaxFRI, nil)

		passed := !analysis.IsUnstable
		sweepRes.Entries = append(sweepRes.Entries, config.SweepEntry{
			Temperature: t,
			Prompt:      targetCfg.Prompt,
			FRI:         analysis.FRI,
			MaxEntropy:  analysis.EntropyProfile.MaxNodeEntropy,
			Archetype:   analysis.Archetype,
			Passed:      passed,
		})

		if analysis.FRI < lowestFRI {
			lowestFRI = analysis.FRI
			sweepRes.OptimalTemperature = t
		}

		if analysis.IsUnstable && sweepRes.PhaseTransitionTemp < 0 {
			sweepRes.PhaseTransitionTemp = t
		}
	}

	config.RenderTerminalSweep(os.Stdout, &sweepRes)
}

func runRecordCommand(args []string) {
	fs := flag.NewFlagSet("record", flag.ExitOnError)
	endpoint := fs.String("endpoint", "", "OpenAI-compatible API endpoint URL")
	apiKey := fs.String("api-key", os.Getenv("OPENAI_API_KEY"), "API key for target endpoint")
	model := fs.String("model", "gpt-4o-mini", "Model identifier")
	prompt := fs.String("prompt", "Execute standard task", "Prompt provided to the agent")
	mockArchetype := fs.String("mock-archetype", "deterministic", "Mock archetype if recording from mock agent")
	outputFile := fs.String("output", "fixture.flakerec.json", "Output file path for fixture")
	runsCount := fs.Int("runs", 5, "Number of runs to profile golden path")

	_ = fs.Parse(args)

	targetType := "mock"
	if *endpoint != "" {
		targetType = "openai"
	}

	orchestrator := runner.NewOrchestrator()
	orchestrator.RegisterAdapter("openai", runner.NewOpenAIAdapter())
	orchestrator.RegisterAdapter("mock", mockagent.NewInProcessMockAdapter())

	suiteConfig := runner.SuiteConfig{
		Target: runner.TargetConfig{
			Type:          targetType,
			Endpoint:      *endpoint,
			APIKey:        *apiKey,
			Model:         *model,
			Prompt:        *prompt,
			MockArchetype: *mockArchetype,
			MaxSteps:      10,
			TimeoutSec:    60,
			Temperature:   0.0,
		},
		TotalRuns:   *runsCount,
		Concurrency: 2,
		MaxFRI:      1.0,
	}

	fmt.Printf("[flakeprobe] Profiling golden trajectory across %d runs...\n", *runsCount)
	runResults, err := orchestrator.RunSuite(context.Background(), suiteConfig, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Profiling failed: %v\n", err)
		os.Exit(1)
	}

	analysis := entropy.AnalyzeFlakiness(runResults, 0.5, nil)
	fixture, err := recorder.SynthesizeFixture(runResults, analysis, *prompt, targetType)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to synthesize fixture: %v\n", err)
		os.Exit(1)
	}

	if err := recorder.SaveFixture(fixture, *outputFile); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save fixture: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[flakeprobe] Golden path serialized with %d steps into %s (path confidence: %.1f%%)\n", fixture.TotalSteps, *outputFile, fixture.PathConfidence*100.0)
}

func runReplayCommand(args []string) {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	fixturePath := fs.String("fixture", "", "Path to .flakerec.json fixture")
	port := fs.Int("port", 8080, "Port to host mock replay server")
	strict := fs.Bool("strict", true, "Enforce strict tool parameter matching")

	_ = fs.Parse(args)

	if *fixturePath == "" {
		fmt.Fprintf(os.Stderr, "Error: --fixture path is required\n")
		os.Exit(1)
	}

	fixture, err := recorder.LoadFixture(*fixturePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load fixture: %v\n", err)
		os.Exit(1)
	}

	server := recorder.NewReplayServer(fixture, *port, *strict)
	fmt.Printf("[flakeprobe] Replay server running on :%d (fixture: %s, steps: %d, strict: %v)\n", *port, *fixturePath, fixture.TotalSteps, *strict)
	fmt.Printf("[flakeprobe] OpenAI completions endpoint: http://localhost:%d/v1/chat/completions\n", *port)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := server.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Replay server stopped: %v\n", err)
		}
	}()

	<-sigChan
	_ = server.Stop(context.Background())
	fmt.Println("[flakeprobe] Replay server stopped.")
}

func runCockpitCommand(args []string) {
	fs := flag.NewFlagSet("cockpit", flag.ExitOnError)
	reportPath := fs.String("report", "", "Path to saved full JSON report")
	port := fs.Int("port", 8484, "Port to host Cockpit dashboard")

	_ = fs.Parse(args)

	var initialReport *report.FullReport
	if *reportPath != "" {
		rep, err := report.LoadReport(*reportPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load report from %s: %v\n", *reportPath, err)
			os.Exit(1)
		}
		initialReport = rep
	} else {
		mockRuns := make([]runner.Run, 15)
		for i := 0; i < 15; i++ {
			mockRuns[i] = *mockagent.GenerateTrajectory(i+1, mockagent.ArchetypeBifurcated, "Sample demo trajectory")
		}
		analysis := entropy.AnalyzeFlakiness(mockRuns, 0.20, nil)
		initialReport = &report.FullReport{
			Timestamp:  time.Now().UTC(),
			TargetDesc: "Demo Agent (Bifurcated Archetype)",
			Analysis:   analysis,
			Runs:       mockRuns,
		}
	}

	srv := cockpit.NewServer(*port, initialReport)
	fmt.Printf("[flakeprobe] Cockpit dashboard listening at http://127.0.0.1:%d\n", *port)
	fmt.Println("[flakeprobe] Press Ctrl+C to stop.")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Cockpit server stopped: %v\n", err)
		}
	}()

	<-sigChan
	_ = srv.Stop(context.Background())
	fmt.Println("[flakeprobe] Cockpit server stopped.")
}

func runMockAgentCommand(args []string) {
	fs := flag.NewFlagSet("mock-agent", flag.ExitOnError)
	port := fs.Int("port", 9090, "Port to host mock agent OpenAI completions server")
	archetype := fs.String("archetype", "bifurcated", "Flakiness archetype: deterministic, low-jitter, bifurcated, hallucination-loop, flaky-failure")

	_ = fs.Parse(args)

	arch := mockagent.Archetype(*archetype)
	srv := mockagent.NewServer(*port, arch)

	fmt.Printf("[flakeprobe] Mock Agent running on :%d (archetype: %s)\n", *port, *archetype)
	fmt.Printf("[flakeprobe] OpenAI endpoint: http://localhost:%d/v1/chat/completions\n", *port)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Mock agent server stopped: %v\n", err)
		}
	}()

	<-sigChan
	_ = srv.Stop(context.Background())
	fmt.Println("[flakeprobe] Mock agent server stopped.")
}
