package runner

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// TargetAdapter defines the interface implemented by various agent target drivers.
type TargetAdapter interface {
	Execute(ctx context.Context, iteration int, config TargetConfig, onProgress func(StepProgress)) (*Run, error)
}

// Orchestrator coordinates concurrent agent runs with controlled jitter.
type Orchestrator struct {
	adapters map[string]TargetAdapter
	mu       sync.RWMutex
}

// NewOrchestrator creates a fresh orchestrator with registered adapters.
func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		adapters: make(map[string]TargetAdapter),
	}
}

// RegisterAdapter registers a driver for a specific target type.
func (o *Orchestrator) RegisterAdapter(targetType string, adapter TargetAdapter) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.adapters[targetType] = adapter
}

// RunSuite executes a suite of N runs across a worker pool.
func (o *Orchestrator) RunSuite(ctx context.Context, config SuiteConfig, onProgress func(StepProgress)) ([]Run, error) {
	o.mu.RLock()
	adapter, exists := o.adapters[config.Target.Type]
	o.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unsupported target type: %s", config.Target.Type)
	}

	concurrency := config.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > config.TotalRuns {
		concurrency = config.TotalRuns
	}

	runs := make([]Run, config.TotalRuns)
	errs := make([]error, config.TotalRuns)

	jobs := make(chan int, config.TotalRuns)
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for iteration := range jobs {
				select {
				case <-ctx.Done():
					errs[iteration] = ctx.Err()
					return
				default:
				}

				if config.JitterMs > 0 {
					jitter := time.Duration(rand.Intn(config.JitterMs)) * time.Millisecond
					time.Sleep(jitter)
				}

				if onProgress != nil {
					onProgress(StepProgress{
						RunID:     fmt.Sprintf("run-%03d", iteration+1),
						Iteration: iteration + 1,
						Status:    "started",
					})
				}

				run, err := adapter.Execute(ctx, iteration+1, config.Target, onProgress)
				if err != nil {
					errs[iteration] = err
					runs[iteration] = Run{
						ID:           fmt.Sprintf("run-%03d", iteration+1),
						Iteration:    iteration + 1,
						StartTime:    time.Now(),
						EndTime:      time.Now(),
						Success:      false,
						ErrorMessage: err.Error(),
					}
					if onProgress != nil {
						onProgress(StepProgress{
							RunID:     fmt.Sprintf("run-%03d", iteration+1),
							Iteration: iteration + 1,
							Status:    "failed",
						})
					}
				} else {
					runs[iteration] = *run
					if onProgress != nil {
						onProgress(StepProgress{
							RunID:     run.ID,
							Iteration: iteration + 1,
							Status:    "completed",
						})
					}
				}
			}
		}(w)
	}

	for i := 0; i < config.TotalRuns; i++ {
		jobs <- i
	}
	close(jobs)

	wg.Wait()

	return runs, nil
}
