package test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/alexandrmotologa/flakeprobe/pkg/cockpit"
	"github.com/alexandrmotologa/flakeprobe/pkg/entropy"
	"github.com/alexandrmotologa/flakeprobe/pkg/mockagent"
	"github.com/alexandrmotologa/flakeprobe/pkg/report"
	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

func TestCockpitInteractiveEndpoints(t *testing.T) {
	runs := make([]runner.Run, 5)
	for i := 0; i < 5; i++ {
		runs[i] = *mockagent.GenerateTrajectory(i+1, mockagent.ArchetypeBifurcated, "test")
	}
	analysis := entropy.AnalyzeFlakiness(runs, 0.20, nil)

	initialReport := &report.FullReport{
		Timestamp:  time.Now().UTC(),
		TargetDesc: "Interactive Test Suite",
		Analysis:   analysis,
		Runs:       runs,
	}

	server := cockpit.NewServer(18494, initialReport)
	go func() {
		_ = server.Start()
	}()
	time.Sleep(50 * time.Millisecond)
	defer func() {
		_ = server.Stop(context.Background())
	}()

	// 1. Test /api/diff
	respDiff, err := http.Get("http://127.0.0.1:18494/api/diff?runA=run-001&runB=run-002")
	if err != nil {
		t.Fatalf("GET /api/diff failed: %v", err)
	}
	defer respDiff.Body.Close()
	if respDiff.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for /api/diff, got %d", respDiff.StatusCode)
	}

	diffBytes, _ := io.ReadAll(respDiff.Body)
	var diffObj map[string]interface{}
	if err := json.Unmarshal(diffBytes, &diffObj); err != nil {
		t.Fatalf("failed to decode /api/diff response: %v", err)
	}
	if diffObj["run_a_id"] != "run-001" {
		t.Errorf("expected run_a_id run-001, got %v", diffObj["run_a_id"])
	}

	// 2. Test /api/suggest
	respSuggest, err := http.Get("http://127.0.0.1:18494/api/suggest")
	if err != nil {
		t.Fatalf("GET /api/suggest failed: %v", err)
	}
	defer respSuggest.Body.Close()
	if respSuggest.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for /api/suggest, got %d", respSuggest.StatusCode)
	}

	// 3. Test POST /api/run
	runReqBody := map[string]interface{}{
		"target_type":    "mock",
		"mock_archetype": "deterministic",
		"runs":           5,
		"concurrency":    2,
		"max_fri":        0.10,
	}
	bodyData, _ := json.Marshal(runReqBody)
	respRun, err := http.Post("http://127.0.0.1:18494/api/run", "application/json", bytes.NewReader(bodyData))
	if err != nil {
		t.Fatalf("POST /api/run failed: %v", err)
	}
	defer respRun.Body.Close()
	if respRun.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for POST /api/run, got %d", respRun.StatusCode)
	}
}
