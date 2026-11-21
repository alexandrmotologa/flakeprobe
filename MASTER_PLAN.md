# flakeprobe: Architecture and Engineering Master Plan
Autonomous AI Agent Flakiness, Path Entropy, and Non-Deterministic Regression Benchmarker
Target: High-Performance Go CLI and Diagnostic Engine (< 20MB RAM) with Embedded Web Cockpit

---

## 1. Problem Statement and Scope

### 1.1 Non-Determinism in Autonomous AI Agent Workflows
Complex LLM-powered autonomous agents (ReAct loops, plan-and-execute architectures, multi-tool agents) exhibit stochastic execution paths. Even when configured with temperature T=0.0, subtle variations in token ordering, upstream API latencies, system prompt slight drifts, or model provider inference-engine concurrency introduce non-deterministic trajectory bifurcation:
1. **Flaky CI/CD Pipelines**: An agent evaluation benchmark passes five times sequentially on a local developer workstation, yet fails in automated CI/CD due to alternative tool call selection.
2. **Hidden Path Divergence**: Two execution runs may produce the identical final text answer while taking radically different tool paths (for example, Run A calls 2 SQL tools costing $0.01; Run B falls into a 12-turn hallucinated web browsing loop costing $0.25).
3. **Lack of Path Entropy Metrics**: Engineering teams have no statistical framework to quantify how fragile an agent's decision boundary is before releasing it to enterprise production.
4. **Mock Brittleness**: Existing test suites attempt to mock individual tool calls, but cannot detect when the agent's internal state machine unpredictably abandons the expected call graph entirely.

### 1.2 The flakeprobe Solution
`flakeprobe` is an autonomous regression testing and trajectory entropy profiler written in pure Go:
- Executes N parallel Monte-Carlo or jittered test trials against agent targets via standardized protocols: OpenAI tool-calling schema, Model Context Protocol (MCP) stdio/SSE endpoints, or generic subprocess CLI runs.
- Extracts full Directed Acyclic Graph (DAG) state transitions for every run: `(State_i -> Action_k -> Tool_j -> Observation_m)`.
- Calculates statistical Path Entropy H(P), Tool Call Variance, and Step Bifurcation Scores across all nodes.
- Computes an overall **Flakiness Risk Index (FRI)** from 0.0 to 1.0 (or 0% to 100%), highlighting the exact decision node responsible for divergence.
- Automatically generates deterministic regression test fixtures (`.flakerec.json`) with deterministic replay mocks for brittle steps.
- Provides a self-contained embedded web cockpit on port 8484 displaying interactive tree overlays, trajectory diff graphs, and entropy heatmaps.
- Gating support for CI/CD pipelines returning non-zero exit codes when FRI exceeds defined safety thresholds.

---

## 2. System Architecture

```
+--------------------------------------------------------------------------+
|                              flakeprobe CLI                              |
|   Targets: OpenAI Tool API | Local Agent CLI | Model Context Protocol    |
+--------------------------------------------------------------------------+
                                     |
                  Target Execution Harness (N Iterations)
                                     v
+--------------------------------------------------------------------------+
|                          Execution Orchestrator                          |
|                                                                          |
|  +--------------------+                    +--------------------------+  |
|  | Concurrency Pool   |------------------->| Perturbation Injector    |  |
|  | (1..32 Workers)    |                    | (Jitter, Seed, Delays)   |  |
|  +--------------------+                    +--------------------------+  |
|                                                         |                |
|                                                         v                |
|  +--------------------------------------------------------------------+  |
|  |                      Trajectory Trace Collector                    |  |
|  |  - Step Index and Timestamp Profiling                              |  |
|  |  - Tool Call Invocation Signature (Name + JSON Args)               |  |
|  |  - Intermediate Reasoning Tokens and Thoughts                      |  |
|  |  - Final State Evaluation and Exit Codes                           |  |
|  +--------------------------------------------------------------------+  |
+------------------------------------|-------------------------------------+
                                     |
                      Traces: [Run_1, Run_2, ..., Run_N]
                                     v
+--------------------------------------------------------------------------+
|                     Statistical Path Entropy Engine                      |
|                                                                          |
|  +-----------------------------+           +--------------------------+  |
|  | Graph Consensus Aligner     |           | Transition Matrix Gen    |  |
|  | (State DAG Reconstruction)  |           | P(Tool_{t+1} | Tool_t)   |  |
|  +-----------------------------+           +--------------------------+  |
|                                                         |                |
|                                                         v                |
|  +--------------------------------------------------------------------+  |
|  |                        Metric Calculation Suite                    |  |
|  |  1. Node Shannon Entropy: H(Node_i)                                |  |
|  |  2. Trajectory Edit Distance (Normalized Levenshtein on Actions)   |  |
|  |  3. Flakiness Risk Index (FRI, 0.0 - 1.0)                          |  |
|  |  4. Tool Argument Jitter and Schema Drift Ratio                    |  |
|  |  5. Flakiness Archetype Classification                             |  |
|  +--------------------------------------------------------------------+  |
+------------------------------------|-------------------------------------+
                                     |
                      Diagnostics & Bifurcation Report
                                     v
+--------------------------------------------------------------------------+
|                       Output & Artifact Generation                       |
|  1. Terminal CLI Summary (Rich Tables, Bifurcation Alerts)               |
|  2. Machine-Readable JSON, JUnit XML, and GitHub Step Summary            |
|  3. Synthesized Replay Mock Contract (.flakerec.json)                    |
|  4. Embedded Web Cockpit (:8484) Interactive Tree & Heatmap              |
+--------------------------------------------------------------------------+
```

---

## 3. Mathematical Formulation

### 3.1 Directed State Transition Graph
Let an agent run $r \in \{1, 2, \dots, N\}$ be represented as an ordered sequence of discrete actions:
$$A^{(r)} = (a_1^{(r)}, a_2^{(r)}, \dots, a_{T_r}^{(r)})$$
where each action $a_t$ corresponds to an invoked tool, reasoning step, or final completion.

The set of all unique states observed across all runs defines the state space $\mathcal{S}$. For any given state $s \in \mathcal{S}$, the empirical probability distribution of transitioning to the next state $s'$ is given by:

$$P(s' \mid s) = \frac{\sum_{r=1}^N \sum_{t=1}^{T_r - 1} \mathbf{1}\left(a_t^{(r)} = s \land a_{t+1}^{(r)} = s'\right)}{\sum_{r=1}^N \sum_{t=1}^{T_r} \mathbf{1}\left(a_t^{(r)} = s\right)}$$

### 3.2 State Bifurcation Entropy ($H(s)$)
The Shannon entropy at state $s$ quantifies the unpredictability of the agent's choice from that decision point:

$$H(s) = -\sum_{s' \in \mathcal{S}} P(s' \mid s) \log_2 P(s' \mid s)$$

- If $H(s) = 0$, the agent deterministically transitions to the same successor across all $N$ runs.
- If $H(s) > 0$, state $s$ represents a bifurcation point. Nodes with $H(s) \ge 1.0$ indicate high decision volatility.

### 3.3 Trajectory Levenshtein Distance & Path Jitter ($\bar{D}$)
Given two action sequences $A^{(i)}$ and $A^{(j)}$, let $D(A^{(i)}, A^{(j)})$ denote the normalized Levenshtein distance over discrete tool tokens:

$$\bar{D} = \frac{2}{N(N - 1)} \sum_{i=1}^{N-1} \sum_{j=i+1}^N \frac{D\left(A^{(i)}, A^{(j)}\right)}{\max(|A^{(i)}|, |A^{(j)}|)}$$

### 3.4 Flakiness Risk Index (FRI)
The composite Flakiness Risk Index blends trajectory distance, outcome variation, and maximum normalized node entropy:

$$\text{FRI} = w_1 \bar{D} + w_2 \left(1 - \frac{|Successes - Failures|}{N}\right) + w_3 \frac{\max_{s} H(s)}{\log_2 |\mathcal{S}_{\text{out}}| + \epsilon}$$

Where default weights are $w_1 = 0.40$, $w_2 = 0.35$, $w_3 = 0.25$. An $\text{FRI} > 0.20$ flags an unstable agent that requires guardrails or prompt restructuring before production rollout.

---

## 4. Subsystems and Architecture

### 4.1 Target Runner Engine (`pkg/runner`)
- **OpenAI Compatible Protocol (`openai.go`)**: Dispatches completions with tool definitions, evaluates tool calls, handles multi-turn loops.
- **Subprocess CLI Protocol (`exec.go`)**: Executes local agent CLI binaries with structured stdout/stderr streaming.
- **MCP Client (`mcp.go`)**: Communicates with Model Context Protocol servers over stdio or SSE JSON-RPC 2.0.
- **Synthetic Mock Agent (`pkg/mockagent`)**: In-process or standalone HTTP server modeling 5 flakiness archetypes for zero-dependency local testing.
- **Worker Pool (`orchestrator.go`)**: Controlled concurrency with jitter injection, retry limits, and trace compilation.

### 4.2 Statistical Graph & Entropy Engine (`pkg/entropy`)
- **Graph Builder (`graph.go`)**: Assembles Directed Acyclic Graphs from execution runs, calculates transition matrices and edge counts.
- **Entropy Calculator (`entropy.go`)**: Computes per-node Shannon entropy $H(s)$, path entropy, and normalized bifurcation metrics.
- **Distance Metrics (`distance.go`)**: Pairwise normalized Levenshtein sequence alignment over action tokens.
- **Semantic Argument Diffing (`argdiff.go`)**: Inspects JSON payloads across runs for schema stability and value jitter, allowing exclusion of volatile keys (like timestamps, UUIDs).
- **Archetype Classifier (`fri.go`)**: Classifies flakiness root causes: `ARGUMENT_DRIFT`, `TOOL_BIFURCATION`, `CYCLE_TRAP`, `OUTCOME_DISCORDANCE`, `LATENCY_SPIKE`.

### 4.3 Deterministic Replay & Contract Synthesizer (`pkg/recorder`)
- **Golden Path Extractor (`record.go`)**: Computes the highest-probability path ($\arg\max \prod P(s' \mid s)$) and serializes state, tool calls, and responses into `.flakerec.json`.
- **Replay Server (`replay.go`)**: Deterministic mock server that validates caller requests against the recorded fixture and responds in < 5ms.

### 4.4 Diagnostic Reporters & CI Gates (`pkg/report`)
- **Terminal Reporter (`terminal.go`)**: Rich tabular format with ANSI colors, bifurcation path highlights, and summary statistics.
- **JSON Reporter (`json.go`)**: Machine-readable payload with full graph nodes, edges, entropy distributions, and run metadata.
- **JUnit XML Reporter (`junit.xml`)**: Test suite XML report compatible with GitHub Actions, GitLab CI, and Jenkins.
- **GitHub Step Summary (`github.go`)**: Formatted Markdown tables output directly to `$GITHUB_STEP_SUMMARY`.

### 4.5 Embedded Web Cockpit (`pkg/cockpit`)
- Zero-dependency embedded web server served on `http://127.0.0.1:8484` using Go's `embed` package.
- Interactive SVG DAG Visualizer: pan, zoom, click to inspect step arguments, entropy heatmap coloring.
- Side-by-side run inspector and trajectory diff.
- Real-time Server-Sent Events (SSE) streaming updates during active test runs.

---

## 5. Command-Line Interface (CLI)

```bash
# Execute 20 iterations against an agent prompt to profile flakiness
flakeprobe test \
  --endpoint "http://localhost:8000/v1" \
  --model "meta-llama/Llama-3.3-70B-Instruct" \
  --prompt "Analyze revenue report and query customer database" \
  --runs 20 \
  --concurrency 5 \
  --max-fri 0.15 \
  --cockpit

# Run regression test against an external CLI agent binary
flakeprobe test \
  --exec "./bin/finance-agent --mode autonomous" \
  --runs 15 \
  --out ./reports/finance_flakiness.json \
  --junit ./reports/junit.xml

# Run test using built-in synthetic mock agent (no API key needed)
flakeprobe test \
  --mock-archetype bifurcated \
  --runs 20 \
  --cockpit

# Start standalone synthetic mock agent
flakeprobe mock-agent --port 9090 --archetype bifurcated

# Generate deterministic replay mock from golden path
flakeprobe record \
  --endpoint "http://localhost:8000/v1" \
  --prompt "Resolve issue 402" \
  --output ./fixtures/issue_402.flakerec.json

# Execute fast deterministic replay in CI/CD pipeline
flakeprobe replay \
  --fixture ./fixtures/issue_402.flakerec.json \
  --port 8080 \
  --strict

# Launch Cockpit dashboard on existing report
flakeprobe cockpit --report ./reports/finance_flakiness.json --port 8484
```

---

## 6. Directory Layout

```
B:\workgit\flakeprobe\
├── cmd/
│   └── flakeprobe/
│       └── main.go                  # CLI entrypoint and subcommand dispatch
├── pkg/
│   ├── runner/
│   │   ├── types.go                 # Trace, Step, Action, Run data structures
│   │   ├── orchestrator.go          # Worker pool and execution coordinator
│   │   ├── openai.go                # OpenAI tool calling harness
│   │   ├── exec.go                  # Subprocess CLI execution harness
│   │   └── mcp.go                   # Model Context Protocol stdio client
│   ├── mockagent/
│   │   ├── server.go                # Synthetic OpenAI and MCP mock agent
│   │   └── archetypes.go            # Deterministic, bifurcated, loop archetypes
│   ├── entropy/
│   │   ├── graph.go                 # Directed state transition graph (DAG)
│   │   ├── entropy.go               # Shannon node entropy H(s) and path entropy
│   │   ├── distance.go              # Normalized Levenshtein sequence distance
│   │   ├── argdiff.go               # Semantic JSON payload argument jitter
│   │   └── fri.go                   # Flakiness Risk Index & archetype classifier
│   ├── recorder/
│   │   ├── record.go                # Golden path extraction & fixture serializer
│   │   └── replay.go                # Deterministic replay mock server
│   ├── report/
│   │   ├── terminal.go              # Console table renderer & color alerts
│   │   ├── json.go                  # JSON report format
│   │   ├── junit.go                 # JUnit XML exporter for CI gates
│   │   └── github.go                # GitHub Step Summary markdown formatter
│   └── cockpit/
│       ├── server.go                # Embedded HTTP server and SSE broadcaster
│       └── web/
│           ├── index.html           # Cockpit HTML single-page layout
│           ├── app.js               # Interactive SVG DAG visualizer and charts
│           └── style.css            # Dark engineering theme styling
├── test/
│   ├── entropy_test.go              # Mathematical unit tests
│   ├── distance_test.go             # Levenshtein distance unit tests
│   ├── recorder_test.go             # Fixture recording and replay tests
│   └── e2e_test.go                  # Full end-to-end integration tests
├── docs/
│   ├── architecture.md              # System design and data flow
│   ├── metrics-guide.md             # Mathematical foundations of FRI and H(s)
│   └── ci-cd-integration.md         # GitHub Actions and GitLab CI guides
├── .github/
│   └── workflows/
│       └── ci.yml                   # Automated testing, linting, and build
├── go.mod
├── go.sum
├── LICENSE
├── README.md
└── MASTER_PLAN.md
```

---

## 7. Implementation Roadmap & Verification Plan

### Phase 1: Repository Foundations and Documentation
- Setup Go module `github.com/alexandrmotologa/flakeprobe`.
- Create `.gitignore`, `LICENSE` (MIT), and humanized technical `README.md`.
- Author in-depth documentation in `docs/` (`architecture.md`, `metrics-guide.md`, `ci-cd-integration.md`).

### Phase 2: Core Domain Models & Runner Harness
- Define core types in `pkg/runner/types.go`.
- Implement `Orchestrator` with worker pool concurrency and jitter management.
- Implement OpenAI tool-calling adapter with multi-turn loop detection.
- Implement subprocess CLI adapter with streaming JSON line parsers.
- Implement Model Context Protocol (MCP) stdio client.

### Phase 3: Synthetic Mock Agent
- Implement configurable mock agent in `pkg/mockagent/server.go`.
- Implement archetypes: `deterministic`, `low-jitter`, `bifurcated`, `hallucination-loop`, `flaky-failure`.
- Enable self-contained CLI usage via `--mock-archetype` and `flakeprobe mock-agent`.

### Phase 4: Mathematical Entropy & Graph Engine
- Implement state transition graph constructor in `pkg/entropy/graph.go`.
- Implement Shannon entropy $H(s)$ in `pkg/entropy/entropy.go`.
- Implement normalized Levenshtein sequence distance in `pkg/entropy/distance.go`.
- Implement JSON semantic diffing and argument jitter in `pkg/entropy/argdiff.go`.
- Implement Flakiness Risk Index (FRI) calculation and root-cause classifier in `pkg/entropy/fri.go`.
- Comprehensive unit tests verifying math against known probability distributions.

### Phase 5: Deterministic Replay & Contract Synthesizer
- Implement golden path extraction in `pkg/recorder/record.go`.
- Implement `.flakerec.json` schema serialization and validation.
- Implement `flakeprobe replay` mock server in `pkg/recorder/replay.go`.
- Unit tests verifying mock replay accuracy and strict matching.

### Phase 6: Reporters and CI Integration
- Implement ANSI terminal table formatting in `pkg/report/terminal.go`.
- Implement JSON exporter in `pkg/report/json.go`.
- Implement JUnit XML exporter in `pkg/report/junit.go`.
- Implement GitHub Step Summary in `pkg/report/github.go`.
- Add threshold exit gating (`--max-fri`) returning non-zero code on violation.

### Phase 7: Embedded Web Cockpit
- Implement zero-dependency HTTP server and SSE broadcaster in `pkg/cockpit/server.go`.
- Author self-contained UI in `pkg/cockpit/web/` (`index.html`, `style.css`, `app.js`).
- Interactive SVG DAG visualizer with node click inspection, entropy heatmaps, and path bifurcation widths.
- Side-by-side run comparison and live stream support.

### Phase 8: CLI Commands Assembly
- Assemble main CLI in `cmd/flakeprobe/main.go` with commands:
  - `flakeprobe test`
  - `flakeprobe record`
  - `flakeprobe replay`
  - `flakeprobe cockpit`
  - `flakeprobe mock-agent`
  - `flakeprobe version`

### Phase 9: Verification, Benchmarks & Memory Validation
- Validate memory consumption under 20MB RSS during 50-run parallel tests.
- Validate analysis latency (< 5ms overhead per 100 traces).
- Full end-to-end integration tests in `test/e2e_test.go`.
- Setup GitHub Actions CI workflow in `.github/workflows/ci.yml`.
