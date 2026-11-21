package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alexandrmotologa/flakeprobe/pkg/entropy"
	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

// FullReport contains the complete exportable test session data.
type FullReport struct {
	Timestamp  time.Time                  `json:"timestamp"`
	TargetDesc string                     `json:"target_description"`
	Analysis   *entropy.FlakinessAnalysis `json:"analysis"`
	Runs       []runner.Run               `json:"runs"`
}

// ExportJSON writes the full session report to a JSON file on disk.
func ExportJSON(filePath string, analysis *entropy.FlakinessAnalysis, runs []runner.Run, targetDesc string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	report := FullReport{
		Timestamp:  time.Now().UTC(),
		TargetDesc: targetDesc,
		Analysis:   analysis,
		Runs:       runs,
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal json report: %w", err)
	}

	return os.WriteFile(filePath, data, 0644)
}

// LoadReport reads a saved FullReport from disk.
func LoadReport(filePath string) (*FullReport, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read report file: %w", err)
	}

	var report FullReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("failed to parse report json: %w", err)
	}

	return &report, nil
}
