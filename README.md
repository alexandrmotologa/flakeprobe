# flakeprobe

AI Agent Flakiness, Path Entropy, and Non-Deterministic Regression Benchmarker in Go.

`flakeprobe` runs N parallel executions against AI agent endpoints, constructs the state transition Directed Acyclic Graph (DAG), calculates Shannon decision entropy, and evaluates the composite Flakiness Risk Index (FRI). It flags decision bifurcations in CI/CD pipelines, synthesizes deterministic replay fixtures (`.flakerec.json`), and provides an embedded real-time web cockpit on port 8484.

---

## The Problem

Autonomous agents (ReAct loops, multi-tool plan-and-execute architectures) frequently exhibit stochastic execution paths. Even when configured with temperature `T=0.0`, differences in upstream API latencies, token ordering, and system prompt formatting cause decision bifurcation:

1. **Flaky CI/CD Pipelines**: Agent benchmarks pass on local machines but fail intermittently in automated CI environments due to alternative tool call selection.
2. **Path Divergence**: Two runs might reach the same final answer while taking different tool paths, multiplying latency and token expenses.
3. **Absence of Path Entropy Metrics**: Teams release agents to production without measuring the fragility of their decision boundaries.
4. **Mock Brittleness**: Traditional unit tests mock single tool responses but fail to detect when the agent state machine abandons the expected trajectory entirely.

---

## Key Capabilities

- **Zero-Dependency Single Binary**: Compiled in Go with memory usage under 20MB RSS during 50-run parallel batches.
- **Multiple Agent Protocols**:
  - OpenAI-compatible tool calling endpoints (`/v1/chat/completions`)
  - External agent CLI subprocesses with JSON-lines streaming
  - Model Context Protocol (MCP) servers over stdio
  - Built-in synthetic mock agent with 5 controllable flakiness archetypes
- **Rigorous Path Entropy Math**:
  - State DAG reconstruction with empirical transition probabilities $P(s' \mid s)$
  - Shannon node bifurcation entropy: $H(s) = -\sum P(s' \mid s) \log_2 P(s' \mid s)$
  - Normalized pairwise Levenshtein distance across action token sequences ($\bar{D}$)
  - Semantic JSON argument drift detection with volatile key filtering
  - Flakiness Risk Index (FRI, 0.0 to 1.0)
- **Deterministic CI Replay Synthesizer**: Extracts the highest-probability golden path into `.flakerec.json` and hosts an in-memory mock server for sub-5ms regression verification.
- **Embedded Web Cockpit (Port 8484)**: Interactive SVG DAG visualizer, node inspection drawers, branch probability callouts, and live Server-Sent Events (SSE).
- **CI Gate Enforcement**: Native exit codes for threshold violations (`--max-fri 0.20`), JUnit XML export, and GitHub Actions step summaries.

---

## Installation

### From Source (Go 1.22+)

```bash
git clone https://github.com/alexandrmotologa/flakeprobe.git
cd flakeprobe
go build -o bin/flakeprobe ./cmd/flakeprobe
```

Verify installation:

```bash
./bin/flakeprobe version
```

---

## Quick Start

### 1. Run Flakiness Benchmark Against an Endpoint

```bash
flakeprobe test \
  --endpoint "http://localhost:8000/v1" \
  --model "meta-llama/Llama-3.3-70B-Instruct" \
  --prompt "Analyze user retention metrics and export summary table" \
  --runs 15 \
  --concurrency 4 \
  --max-fri 0.20 \
  --cockpit
```

### 2. Test Using the Built-in Synthetic Agent

Run a test with zero external dependencies using built-in archetypes (`deterministic`, `low-jitter`, `bifurcated`, `hallucination-loop`, `flaky-failure`):

```bash
flakeprobe test \
  --mock-archetype bifurcated \
  --runs 20 \
  --max-fri 0.20 \
  --out ./reports/bifurcated.json \
  --junit ./reports/junit.xml
```

Sample terminal output:

```text
=== flakeprobe: Agent Reliability & Path Entropy Report ===
Target:     Built-in Mock Agent (archetype: bifurcated)
Status:     [FAIL: UNSTABLE AGENT] (Threshold: 0.20)
Archetype:  TOOL_BIFURCATION
--------------------------------------------------------------------
METRIC                           VALUE            EVALUATION      
--------------------------------------------------------------------
Flakiness Risk Index (FRI)       0.2636           Exceeds Gate    
Mean Trajectory Distance (D)     0.3556           Divergent       
Max Node Entropy H(s)            0.9710           Bifurcated      
Argument Drift Score             0.00                             
Run Success Rate                 100.0%                           
Unique Execution Paths           2                                
Total Sampled Iterations         10                               
--------------------------------------------------------------------

Bifurcation Bottlenecks Detected (1):
  * Step -1 [Start] -> H(s) = 0.971 bits
      |- step_0:vector_search         (count:  4, prob: 40.0%)
      |- step_0:query_sql             (count:  6, prob: 60.0%)

Diagnostics & Recommendations:
  - Agent branches into divergent tool pathways at decision points. Add explicit disambiguation instructions in system prompt.

[flakeprobe] GATE FAILED: Flakiness Risk Index (0.2636) exceeded threshold (0.2000).
```

### 3. Synthesize Deterministic Replay Contract

Capture the highest-probability path and save it as an offline regression fixture:

```bash
flakeprobe record \
  --endpoint "http://localhost:8000/v1" \
  --prompt "Process payment verification" \
  --output ./fixtures/payment.flakerec.json \
  --runs 5
```

### 4. Execute Fast Deterministic Replay in CI/CD

Serve the recorded fixture as an instant mock server during integration tests:

```bash
flakeprobe replay \
  --fixture ./fixtures/payment.flakerec.json \
  --port 8080 \
  --strict
```

### 5. Launch Cockpit on Existing Report

```bash
flakeprobe cockpit --report ./reports/bifurcated.json --port 8484
```

Then open `http://127.0.0.1:8484` in your browser.

---

## Mathematical Formulation

### State Bifurcation Entropy ($H(s)$)

Given empirical transition probabilities $P(s' \mid s)$ from state $s$ to successor states $s'$:

$$H(s) = -\sum_{s' \in \mathcal{S}} P(s' \mid s) \log_2 P(s' \mid s)$$

- $H(s) = 0.0$: Transition is deterministic across all sampled runs.
- $0.0 < H(s) < 1.0$: Minor branching observed.
- $H(s) \ge 1.0$: Critical decision volatility.

### Normalized Trajectory Distance ($\bar{D}$)

Normalized pairwise Levenshtein distance across discrete action tokens:

$$\bar{D} = \frac{2}{N(N - 1)} \sum_{i=1}^{N-1} \sum_{j=i+1}^N \frac{D\left(A^{(i)}, A^{(j)}\right)}{\max(|A^{(i)}|, |A^{(j)}|)}$$

### Flakiness Risk Index (FRI)

$$\text{FRI} = w_1 \bar{D} + w_2 \left(1 - \frac{|Successes - Failures|}{N}\right) + w_3 \frac{\max_s H(s)}{\log_2 |\mathcal{S}_{\text{out}}| + \epsilon}$$

Default weights: $w_1 = 0.40$ (trajectory distance), $w_2 = 0.35$ (outcome variance), $w_3 = 0.25$ (maximum node entropy). An agent with $\text{FRI} > 0.20$ is flagged as unstable.

---

## CLI Reference

| Command | Key Flags | Description |
| :--- | :--- | :--- |
| `test` | `--endpoint`, `--prompt`, `--runs`, `--max-fri`, `--cockpit` | Runs N trials, computes entropy metrics, evaluates pass/fail gate |
| `record` | `--endpoint`, `--prompt`, `--output`, `--runs` | Profiles golden trajectory and outputs `.flakerec.json` |
| `replay` | `--fixture`, `--port`, `--strict` | Hosts deterministic mock HTTP server from fixture |
| `cockpit` | `--report`, `--port` | Serves embedded web visualization dashboard |
| `mock-agent` | `--port`, `--archetype` | Runs OpenAI-compatible synthetic mock server |
| `version` | - | Displays binary version information |

---

## Documentation

- [Architecture Design](docs/architecture.md)
- [Metrics & Mathematical Guide](docs/metrics-guide.md)
- [CI/CD Integration Guide](docs/ci-cd-integration.md)

---

## License

MIT License. See [LICENSE](LICENSE) for details.
