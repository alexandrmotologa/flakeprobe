package entropy

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

// ArgumentDriftProfile summarizes stability of JSON parameters across tool invocations.
type ArgumentDriftProfile struct {
	OverallDriftScore float64             `json:"overall_drift_score"` // 0.0 = perfectly identical, 1.0 = completely fluctuating
	ToolAnalyses      []ToolArgStability  `json:"tool_analyses"`
	VolatileKeys      []string            `json:"volatile_keys"`
}

// ToolArgStability details argument consistency for a specific tool at a step index.
type ToolArgStability struct {
	ToolName        string   `json:"tool_name"`
	StepIndex       int      `json:"step_index"`
	TotalCalls      int      `json:"total_calls"`
	DistinctPayloads int     `json:"distinct_payloads"`
	DriftScore      float64  `json:"drift_score"`
	FluctuatingKeys []string `json:"fluctuating_keys"`
}

// ComputeArgumentDrift examines tool arguments across runs, discounting specified volatile keys.
func ComputeArgumentDrift(runs []runner.Run, volatileKeys []string) *ArgumentDriftProfile {
	defaultVolatile := map[string]bool{
		"timestamp":  true,
		"time":       true,
		"date":       true,
		"nonce":      true,
		"id":         true,
		"uuid":       true,
		"request_id": true,
		"trace_id":   true,
	}

	for _, k := range volatileKeys {
		defaultVolatile[strings.ToLower(k)] = true
	}

	// Group steps by tool and step index
	type toolKey struct {
		name string
		step int
	}

	groupedArgs := make(map[toolKey][]map[string]interface{})

	for _, run := range runs {
		for _, step := range run.Steps {
			if step.ActionType == "tool_call" && step.ToolName != "" && step.Arguments != nil {
				tk := toolKey{name: step.ToolName, step: step.Index}
				// Clean volatile keys
				cleaned := cleanVolatile(step.Arguments, defaultVolatile)
				groupedArgs[tk] = append(groupedArgs[tk], cleaned)
			}
		}
	}

	var analyses []ToolArgStability
	totalDrift := 0.0

	for tk, argList := range groupedArgs {
		if len(argList) <= 1 {
			continue
		}

		distinct := 0
		seen := make([]map[string]interface{}, 0)
		fluctuating := make(map[string]bool)

		for _, argMap := range argList {
			matched := false
			for _, s := range seen {
				if reflect.DeepEqual(argMap, s) {
					matched = true
					break
				}
			}
			if !matched {
				seen = append(seen, argMap)
				distinct++
			}
		}

		// Find fluctuating keys
		if len(seen) > 1 {
			base := seen[0]
			for i := 1; i < len(seen); i++ {
				for k, v := range seen[i] {
					if baseVal, ok := base[k]; !ok || !reflect.DeepEqual(baseVal, v) {
						fluctuating[k] = true
					}
				}
				for k := range base {
					if _, ok := seen[i][k]; !ok {
						fluctuating[k] = true
					}
				}
			}
		}

		driftScore := 0.0
		if len(argList) > 1 {
			driftScore = float64(distinct-1) / float64(len(argList)-1)
		}

		var keys []string
		for k := range fluctuating {
			keys = append(keys, k)
		}

		analyses = append(analyses, ToolArgStability{
			ToolName:         tk.name,
			StepIndex:        tk.step,
			TotalCalls:       len(argList),
			DistinctPayloads: distinct,
			DriftScore:       driftScore,
			FluctuatingKeys:  keys,
		})

		totalDrift += driftScore
	}

	overallScore := 0.0
	if len(analyses) > 0 {
		overallScore = totalDrift / float64(len(analyses))
	}

	var volList []string
	for k := range defaultVolatile {
		volList = append(volList, k)
	}

	return &ArgumentDriftProfile{
		OverallDriftScore: overallScore,
		ToolAnalyses:      analyses,
		VolatileKeys:      volList,
	}
}

func cleanVolatile(orig map[string]interface{}, volatile map[string]bool) map[string]interface{} {
	cleaned := make(map[string]interface{})
	for k, v := range orig {
		if volatile[strings.ToLower(k)] {
			continue
		}
		// Deep clean nested maps if any
		if nestedMap, ok := v.(map[string]interface{}); ok {
			cleaned[k] = cleanVolatile(nestedMap, volatile)
		} else {
			cleaned[k] = fmt.Sprintf("%v", v)
		}
	}
	return cleaned
}
