package mockagent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

// Archetype defines the behavioral flakiness profile of a mock agent.
type Archetype string

const (
	ArchetypeDeterministic     Archetype = "deterministic"
	ArchetypeLowJitter         Archetype = "low-jitter"
	ArchetypeBifurcated        Archetype = "bifurcated"
	ArchetypeHallucinationLoop Archetype = "hallucination-loop"
	ArchetypeFlakyFailure      Archetype = "flaky-failure"
)

// GenerateTrajectory generates a realistic synthetic run trajectory according to the specified archetype.
func GenerateTrajectory(iteration int, archetype Archetype, prompt string) *runner.Run {
	runID := fmt.Sprintf("run-%03d", iteration)
	startTime := time.Now()
	var steps []runner.Step
	totalTokens := 0
	success := true
	finalOutput := ""

	switch archetype {
	case ArchetypeDeterministic:
		// Exact 2-tool deterministic trajectory
		steps = append(steps, runner.Step{
			Index:      0,
			Timestamp:  startTime,
			DurationMs: 45,
			ActionType: "tool_call",
			ToolName:   "read_schema",
			Arguments: map[string]interface{}{
				"database": "production_analytics",
				"table":    "users",
			},
			Observation: `{"columns":["id","email","created_at","tier"]}`,
		})
		totalTokens += 150

		steps = append(steps, runner.Step{
			Index:      1,
			Timestamp:  startTime.Add(50 * time.Millisecond),
			DurationMs: 65,
			ActionType: "tool_call",
			ToolName:   "query_db",
			Arguments: map[string]interface{}{
				"sql": "SELECT count(*) AS total FROM users WHERE tier = 'enterprise'",
			},
			Observation: `{"rows":[{"total":482}]}`,
		})
		totalTokens += 220

		steps = append(steps, runner.Step{
			Index:       2,
			Timestamp:   startTime.Add(120 * time.Millisecond),
			DurationMs:  30,
			ActionType:  "completion",
			Observation: "There are currently 482 active enterprise accounts.",
			Thought:     "I have verified the database count and formulated the response.",
			IsTerminal:  true,
		})
		totalTokens += 80
		finalOutput = "There are currently 482 active enterprise accounts."

	case ArchetypeLowJitter:
		// Same sequence of tools, but arguments have variable limits and timestamp jitter
		limit := 50 + (iteration % 3)*25
		bytes := make([]byte, 4)
		_, _ = rand.Read(bytes)
		nonce := hex.EncodeToString(bytes)

		steps = append(steps, runner.Step{
			Index:      0,
			Timestamp:  startTime,
			DurationMs: 40,
			ActionType: "tool_call",
			ToolName:   "fetch_metrics",
			Arguments: map[string]interface{}{
				"metric": "cpu_utilization",
				"limit":  limit,
				"nonce":  nonce,
			},
			Observation: fmt.Sprintf(`{"samples":%d,"mean":64.2}`, limit),
		})
		totalTokens += 160

		steps = append(steps, runner.Step{
			Index:      1,
			Timestamp:  startTime.Add(45 * time.Millisecond),
			DurationMs: 55,
			ActionType: "tool_call",
			ToolName:   "format_chart",
			Arguments: map[string]interface{}{
				"type":  "timeseries",
				"title": "CPU Metrics",
			},
			Observation: `{"chart_url":"https://charts.local/cpu_991"}`,
		})
		totalTokens += 210

		steps = append(steps, runner.Step{
			Index:       2,
			Timestamp:   startTime.Add(105 * time.Millisecond),
			DurationMs:  25,
			ActionType:  "completion",
			Observation: "Generated CPU utilization chart with 64.2% mean usage.",
			IsTerminal:  true,
		})
		totalTokens += 60
		finalOutput = "Generated CPU utilization chart."

	case ArchetypeBifurcated:
		// 50% chance Branch A (SQL), 50% chance Branch B (Vector Search)
		coin, _ := rand.Int(rand.Reader, big.NewInt(2))
		if coin.Int64() == 0 {
			// Branch A
			steps = append(steps, runner.Step{
				Index:      0,
				Timestamp:  startTime,
				DurationMs: 50,
				ActionType: "tool_call",
				ToolName:   "query_sql",
				Arguments: map[string]interface{}{
					"query": "SELECT * FROM tickets WHERE status='open'",
				},
				Observation: `{"tickets_count":12}`,
			})
			totalTokens += 180

			steps = append(steps, runner.Step{
				Index:      1,
				Timestamp:  startTime.Add(55 * time.Millisecond),
				DurationMs: 40,
				ActionType: "tool_call",
				ToolName:   "format_table",
				Arguments: map[string]interface{}{
					"style": "compact",
				},
				Observation: `{"rendered":true}`,
			})
			totalTokens += 140

			steps = append(steps, runner.Step{
				Index:       2,
				Timestamp:   startTime.Add(100 * time.Millisecond),
				DurationMs:  20,
				ActionType:  "completion",
				Observation: "Found 12 open tickets via SQL query.",
				IsTerminal:  true,
			})
			totalTokens += 70
			finalOutput = "Found 12 open tickets via SQL query."
		} else {
			// Branch B
			steps = append(steps, runner.Step{
				Index:      0,
				Timestamp:  startTime,
				DurationMs: 65,
				ActionType: "tool_call",
				ToolName:   "vector_search",
				Arguments: map[string]interface{}{
					"index": "customer_support",
					"top_k": 5,
				},
				Observation: `{"matches":["issue-101","issue-102"]}`,
			})
			totalTokens += 260

			steps = append(steps, runner.Step{
				Index:      1,
				Timestamp:  startTime.Add(70 * time.Millisecond),
				DurationMs: 50,
				ActionType: "tool_call",
				ToolName:   "summarize_docs",
				Arguments: map[string]interface{}{
					"max_length": 200,
				},
				Observation: `{"summary":"Open tickets relate to billing questions."}`,
			})
			totalTokens += 200

			steps = append(steps, runner.Step{
				Index:       2,
				Timestamp:   startTime.Add(125 * time.Millisecond),
				DurationMs:  25,
				ActionType:  "completion",
				Observation: "Customer support tickets analyzed via semantic search.",
				IsTerminal:  true,
			})
			totalTokens += 90
			finalOutput = "Customer support tickets analyzed via semantic search."
		}

	case ArchetypeHallucinationLoop:
		// 50% clean path, 50% loop trap
		coin, _ := rand.Int(rand.Reader, big.NewInt(2))
		if coin.Int64() == 0 {
			steps = append(steps, runner.Step{
				Index:      0,
				Timestamp:  startTime,
				DurationMs: 35,
				ActionType: "tool_call",
				ToolName:   "lookup_record",
				Arguments: map[string]interface{}{
					"id": "item_900",
				},
				Observation: `{"found":true,"name":"Server Alpha"}`,
			})
			totalTokens += 120

			steps = append(steps, runner.Step{
				Index:       1,
				Timestamp:   startTime.Add(40 * time.Millisecond),
				DurationMs:  20,
				ActionType:  "completion",
				Observation: "Record item_900 found.",
				IsTerminal:  true,
			})
			totalTokens += 50
			finalOutput = "Record item_900 found."
		} else {
			// Loop trap (3 retries)
			for l := 0; l < 3; l++ {
				steps = append(steps, runner.Step{
					Index:      l,
					Timestamp:  startTime.Add(time.Duration(l*40) * time.Millisecond),
					DurationMs: 35,
					ActionType: "tool_call",
					ToolName:   "retry_search",
					Arguments: map[string]interface{}{
						"query": fmt.Sprintf("attempt_%d", l+1),
					},
					Observation: `{"found":false,"error":"rate_limit_exceeded"}`,
				})
				totalTokens += 190
			}

			steps = append(steps, runner.Step{
				Index:       3,
				Timestamp:   startTime.Add(130 * time.Millisecond),
				DurationMs:  20,
				ActionType:  "completion",
				Observation: "Aborted search after 3 attempts.",
				IsTerminal:  true,
			})
			totalTokens += 60
			finalOutput = "Search aborted."
		}

	case ArchetypeFlakyFailure:
		// 70% success, 30% failure requiring fallback
		coin, _ := rand.Int(rand.Reader, big.NewInt(10))
		if coin.Int64() < 7 {
			steps = append(steps, runner.Step{
				Index:      0,
				Timestamp:  startTime,
				DurationMs: 40,
				ActionType: "tool_call",
				ToolName:   "call_api",
				Arguments: map[string]interface{}{
					"endpoint": "/v1/payment/verify",
				},
				Observation: `{"status":"authorized"}`,
			})
			totalTokens += 150

			steps = append(steps, runner.Step{
				Index:       1,
				Timestamp:   startTime.Add(45 * time.Millisecond),
				DurationMs:  25,
				ActionType:  "completion",
				Observation: "Payment verified successfully.",
				IsTerminal:  true,
			})
			totalTokens += 55
			finalOutput = "Payment verified."
		} else {
			steps = append(steps, runner.Step{
				Index:      0,
				Timestamp:  startTime,
				DurationMs: 40,
				ActionType: "tool_call",
				ToolName:   "call_api",
				Arguments: map[string]interface{}{
					"endpoint": "/v1/payment/verify",
				},
				Observation: `{"error":"gateway_timeout"}`,
				Error:       "gateway_timeout",
			})
			totalTokens += 150

			steps = append(steps, runner.Step{
				Index:      1,
				Timestamp:  startTime.Add(45 * time.Millisecond),
				DurationMs: 30,
				ActionType: "tool_call",
				ToolName:   "fallback_cache",
				Arguments: map[string]interface{}{
					"key": "payment_status_cached",
				},
				Observation: `{"cached":false}`,
			})
			totalTokens += 130

			steps = append(steps, runner.Step{
				Index:       2,
				Timestamp:   startTime.Add(80 * time.Millisecond),
				DurationMs:  20,
				ActionType:  "completion",
				Observation: "Payment verification failed with gateway timeout.",
				IsTerminal:  true,
			})
			totalTokens += 60
			success = false
			finalOutput = "Payment verification failed."
		}
	}

	endTime := time.Now()
	return &runner.Run{
		ID:          runID,
		Iteration:   iteration,
		StartTime:   startTime,
		EndTime:     endTime,
		DurationMs:  endTime.Sub(startTime).Milliseconds(),
		Steps:       steps,
		TotalTokens: totalTokens,
		Success:     success,
		FinalOutput: finalOutput,
		Metadata: map[string]interface{}{
			"archetype": string(archetype),
			"prompt":    prompt,
		},
	}
}
