package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexandrmotologa/flakeprobe/pkg/entropy"
	"github.com/alexandrmotologa/flakeprobe/pkg/mockagent"
	"github.com/alexandrmotologa/flakeprobe/pkg/recorder"
	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

func TestSynthesizeAndVerifyFixture(t *testing.T) {
	runs := make([]runner.Run, 5)
	for i := 0; i < 5; i++ {
		runs[i] = *mockagent.GenerateTrajectory(i+1, mockagent.ArchetypeDeterministic, "test recording")
	}

	analysis := entropy.AnalyzeFlakiness(runs, 0.20, nil)
	fixture, err := recorder.SynthesizeFixture(runs, analysis, "test recording", "mock")
	if err != nil {
		t.Fatalf("failed to synthesize fixture: %v", err)
	}

	if fixture.TotalSteps != 3 {
		t.Errorf("expected 3 steps in fixture, got %d", fixture.TotalSteps)
	}

	if fixture.Checksum == "" {
		t.Errorf("expected non-empty checksum")
	}

	tmpFile := filepath.Join(t.TempDir(), "test.flakerec.json")
	if err := recorder.SaveFixture(fixture, tmpFile); err != nil {
		t.Fatalf("failed to save fixture: %v", err)
	}

	loaded, err := recorder.LoadFixture(tmpFile)
	if err != nil {
		t.Fatalf("failed to load fixture: %v", err)
	}

	if loaded.Checksum != fixture.Checksum {
		t.Errorf("checksum mismatch: %s != %s", loaded.Checksum, fixture.Checksum)
	}

	// Verify exact match against golden run
	res := recorder.VerifyRunAgainstFixture(runs[0], *loaded, true)
	if !res.Matched {
		t.Errorf("expected run to match fixture, but had differences: %v", res.Differences)
	}

	// Create divergent run with altered tool name
	divergentRun := runs[0]
	divergentRun.Steps[0].ToolName = "altered_tool"
	resDivergent := recorder.VerifyRunAgainstFixture(divergentRun, *loaded, true)
	if resDivergent.Matched {
		t.Errorf("expected divergent run to fail verification")
	}
	_ = os.Remove(tmpFile)
}
