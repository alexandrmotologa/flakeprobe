package test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/alexandrmotologa/flakeprobe/pkg/cockpit"
	"github.com/alexandrmotologa/flakeprobe/pkg/entropy"
	"github.com/alexandrmotologa/flakeprobe/pkg/report"
	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

func TestCockpitServerEndpoints(t *testing.T) {
	runs := []runner.Run{
		{
			ID: "run-001",
			Steps: []runner.Step{
				{Index: 0, ActionType: "tool_call", ToolName: "test_tool"},
				{Index: 1, ActionType: "completion", IsTerminal: true},
			},
			Success: true,
		},
	}
	analysis := entropy.AnalyzeFlakiness(runs, 0.20, nil)

	initialReport := &report.FullReport{
		Timestamp:  time.Now().UTC(),
		TargetDesc: "Test Suite Target",
		Analysis:   analysis,
		Runs:       runs,
	}

	server := cockpit.NewServer(18493, initialReport)
	go func() {
		_ = server.Start()
	}()
	time.Sleep(50 * time.Millisecond)
	defer func() {
		_ = server.Stop(context.Background())
	}()

	// 1. Test index.html
	respIndex, err := http.Get("http://127.0.0.1:18493/")
	if err != nil {
		t.Fatalf("failed to GET index.html: %v", err)
	}
	defer respIndex.Body.Close()
	if respIndex.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for index.html, got %d", respIndex.StatusCode)
	}

	// 2. Test /api/report
	respReport, err := http.Get("http://127.0.0.1:18493/api/report")
	if err != nil {
		t.Fatalf("failed to GET /api/report: %v", err)
	}
	defer respReport.Body.Close()
	if respReport.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for /api/report, got %d", respReport.StatusCode)
	}

	body, _ := io.ReadAll(respReport.Body)
	var loadedReport report.FullReport
	if err := json.Unmarshal(body, &loadedReport); err != nil {
		t.Fatalf("failed to parse /api/report body: %v", err)
	}

	if loadedReport.TargetDesc != "Test Suite Target" {
		t.Errorf("target description mismatch: %s", loadedReport.TargetDesc)
	}
}
