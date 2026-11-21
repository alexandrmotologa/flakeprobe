# flakeprobe Architecture

This document describes the software design and components of flakeprobe.

## Overview

flakeprobe evaluates non-deterministic execution in autonomous AI agents. It coordinates test executions across concurrent workers, normalizes captured steps into discrete action tokens, builds a directed state transition graph, and computes entropy metrics.

## Subsystems

### 1. Runner Engine (`pkg/runner`)
- `Orchestrator`: Dispatches N iterations across a configurable worker pool. Injects optional inter-run jitter.
- `OpenAIAdapter`: Interacts with OpenAI-compatible chat completion endpoints. Sends tool definitions, captures tool invocation steps, and feeds tool observations back until a terminal response is reached.
- `ExecAdapter`: Spawns external CLI processes and parses line-delimited JSON events representing actions, observations, and token usage.
- `MCPAdapter`: Connects to Model Context Protocol servers over stdio, initiates JSON-RPC 2.0 handshake, discovers tools via `tools/list`, and executes tool calls.

### 2. Synthetic Mock Agent (`pkg/mockagent`)
- Provides deterministic and stochastic agent profiles without external network requirements.
- Supports five behavioral archetypes:
  - `deterministic`: Fixed sequence with constant parameters.
  - `low-jitter`: Fixed sequence with variable argument values.
  - `bifurcated`: Stochastic branch between two distinct tool sequences.
  - `hallucination-loop`: Variable retry loops and iteration depths.
  - `flaky-failure`: Stochastic failures requiring fallback tool branches.

### 3. Entropy Engine (`pkg/entropy`)
- `BuildStateDAG`: Constructs a directed acyclic graph where nodes represent discrete states and edges represent observed transitions.
- `ComputeDAGEntropy`: Calculates Shannon entropy for decision nodes with multiple outgoing branches.
- `ComputeTrajectoryDistances`: Calculates pairwise normalized Levenshtein edit distance across action sequences.
- `ComputeArgumentDrift`: Compares JSON arguments for identical tools, discounting volatile fields such as timestamps and nonces.
- `AnalyzeFlakiness`: Computes the composite Flakiness Risk Index (FRI) and categorizes the observed flakiness archetype.

### 4. Recorder & Replay (`pkg/recorder`)
- `SynthesizeFixture`: Selects the highest-probability path and serializes it to `.flakerec.json`.
- `ReplayServer`: Serves responses from the recorded fixture over HTTP for deterministic offline regression testing.

### 5. Diagnostics & Cockpit (`pkg/report`, `pkg/cockpit`)
- Terminal tabular scorecard with ANSI coloring.
- JSON and JUnit XML exports for CI pipeline gates.
- Embedded web dashboard on port 8484 serving an interactive SVG visualization of the state transition graph.
