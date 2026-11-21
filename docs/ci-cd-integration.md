# CI/CD Integration Guide

This guide explains how to integrate flakeprobe into automated CI/CD pipelines.

## GitHub Actions

Use flakeprobe in GitHub Actions to catch agent regressions and path flakiness on pull requests.

```yaml
name: Agent Reliability Gate

on:
  pull_request:
    branches: [main]

jobs:
  flakeprobe:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Build flakeprobe
        run: go build -o bin/flakeprobe ./cmd/flakeprobe

      - name: Run Flakiness Benchmark
        run: |
          ./bin/flakeprobe test \
            --endpoint "http://localhost:8000/v1" \
            --prompt "Process incoming invoice" \
            --runs 20 \
            --concurrency 4 \
            --max-fri 0.15 \
            --junit ./reports/junit.xml \
            --out ./reports/report.json

      - name: Publish Test Results
        uses: EnricoMi/publish-unit-test-result-action@v2
        if: always()
        with:
          files: reports/junit.xml
```

## Exit Code Semantics

- `0`: Agent passed the flakiness gate (FRI is below threshold).
- `1`: Agent failed the flakiness gate (FRI exceeded threshold) or execution error occurred.
