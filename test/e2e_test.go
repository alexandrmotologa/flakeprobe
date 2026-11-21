package test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/alexandrmotologa/flakeprobe/pkg/entropy"
	"github.com/alexandrmotologa/flakeprobe/pkg/mockagent"
	"github.com/alexandrmotologa/flakeprobe/pkg/recorder"
	"github.com/alexandrmotologa/flakeprobe/pkg/report"
	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

func TestEndToEndPipeline(t *testing.T) {
	orchestrator := runner.NewOrchestrator()
	orchestrator.RegisterAdapter("mock", mockagent.NewInProcessMockAdapter())

	suiteConfig := runner.SuiteConfig{
		Target: runner.TargetConfig{
			Type:          "mock",
			MockArchetype: "bifurcated",
			Prompt:        "Query user records and generate report",
		},
		TotalRuns:   20,
		Concurrency: 4,
		MaxFRI:      0.25,
	}

	// Measure memory before
	runtime.GC()
	var mBefore runtime.MemStats
	runtime.ReadMemStats(&mBefore)

	startTime := time.Now()
	runs, err := orchestrator.RunSuite(context.Background(), suiteConfig, nil)
	if err != nil {
		t.Fatalf("orchestrator RunSuite failed: %v", err)
	}

	if len(runs) != 20 {
		t.Fatalf("expected 20 runs, got %d", len(runs))
	}

	// Analyze flakiness
	analysisStart := time.Now()
	analysis := entropy.AnalyzeFlakiness(runs, 0.25, nil)
	analysisDur := time.Since(analysisStart)

	t.Logf("Flakiness analysis completed in %v (FRI: %.4f, Archetype: %s)", analysisDur, analysis.FRI, analysis.Archetype)

	// Verify memory stays low (< 20MB alloc)
	var mAfter runtime.MemStats
	runtime.ReadMemStats(&mAfter)
	allocatedMB := float64(mAfter.Alloc-mBefore.Alloc) / (1024 * 1024)
	t.Logf("Memory delta during test: %.2f MB", allocatedMB)

	if allocatedMB > 20.0 {
		t.Errorf("allocated memory exceeds 20MB threshold: %.2f MB", allocatedMB)
	}

	// Verify analysis duration (< 20ms)
	if analysisDur > 20*time.Millisecond {
		t.Errorf("analysis took longer than 20ms: %v", analysisDur)
	}

	// Verify reports export cleanly
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "report.json")
	junitPath := filepath.Join(tmpDir, "junit.xml")
	fixturePath := filepath.Join(tmpDir, "test.flakerec.json")

	if err := report.ExportJSON(jsonPath, analysis, runs, "Mock Agent Bifurcated"); err != nil {
		t.Errorf("failed to export JSON report: %v", err)
	}

	if err := report.ExportJUnitXML(junitPath, analysis, "Mock Agent Bifurcated"); err != nil {
		t.Errorf("failed to export JUnit report: %v", err)
	}

	fixture, err := recorder.SynthesizeFixture(runs, analysis, suiteConfig.Target.Prompt, "mock")
	if err != nil {
		t.Errorf("failed to synthesize fixture: %v", err)
	} else if err := recorder.SaveFixture(fixture, fixturePath); err != nil {
		t.Errorf("failed to save fixture: %v", err)
	}

	// Confirm files exist and are not empty
	for _, p := range []string{jsonPath, junitPath, fixturePath} {
		fi, err := os.Stat(p)
		if err != nil || fi.Size() == 0 {
			t.Errorf("expected valid non-empty file at %s", p)
		}
	}

	totalDur := time.Since(startTime)
	t.Logf("Total 20-run execution & pipeline completed in %v", totalDur)
}
