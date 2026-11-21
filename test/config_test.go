package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexandrmotologa/flakeprobe/pkg/config"
)

func TestLoadConfigJSON(t *testing.T) {
	jsonContent := `{
		"version": "1",
		"target": {
			"type": "mock",
			"mock_archetype": "bifurcated",
			"prompt": "Test prompt"
		},
		"suite": {
			"runs": 15,
			"concurrency": 3,
			"max_fri": 0.15
		}
	}`

	tmpFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(tmpFile, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Suite.Runs != 15 {
		t.Errorf("expected 15 runs, got %d", cfg.Suite.Runs)
	}
	if cfg.Suite.MaxFRI != 0.15 {
		t.Errorf("expected max_fri 0.15, got %f", cfg.Suite.MaxFRI)
	}
	if cfg.Target.MockArchetype != "bifurcated" {
		t.Errorf("expected mock archetype bifurcated, got %s", cfg.Target.MockArchetype)
	}
}

func TestLoadConfigYAML(t *testing.T) {
	yamlContent := `
target:
  type: mock
  mock_archetype: deterministic
  prompt: "Process records"
suite:
  runs: 25
  concurrency: 5
  max_fri: 0.10
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write yaml config file: %v", err)
	}

	cfg, err := config.LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("failed to load yaml config: %v", err)
	}

	if cfg.Suite.Runs != 25 {
		t.Errorf("expected 25 runs, got %d", cfg.Suite.Runs)
	}
	if cfg.Suite.Concurrency != 5 {
		t.Errorf("expected concurrency 5, got %d", cfg.Suite.Concurrency)
	}
}
