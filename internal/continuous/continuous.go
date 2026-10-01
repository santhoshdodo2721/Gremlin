package continuous

import (
	"context"
	"fmt"
	"time"

	"gremlin-in-a-box/internal/plugins"
)

type Step struct {
	Attack string
	Params plugins.Params
}
type Summary struct{ Cycles, Passed, Failed, Interrupted int }
type Event struct {
	Cycle   int
	Attack  string
	Elapsed time.Duration
	Err     error
	Summary Summary
}
type AttackFunc func(context.Context, string, string, plugins.Params, string) error

// Run performs serial experiments. The next fault starts only after recovery and
// the configured quiet period. A failed experiment stops the run for inspection.
func Run(ctx context.Context, attack AttackFunc, target string, plan []Step, healthURL string, gap time.Duration, cycles int, result func(Event)) (Summary, error) {
	summary := Summary{}
	if len(plan) == 0 || gap <= 0 || cycles < 0 || target == "" {
		return summary, fmt.Errorf("continuous run needs a target, tests, positive interval, and nonnegative cycles")
	}
	for cycle := 1; cycles == 0 || cycle <= cycles; cycle++ {
		for i, step := range plan {
			if err := ctx.Err(); err != nil {
				return summary, err
			}
			start := time.Now()
			err := attack(ctx, step.Attack, target, step.Params, healthURL)
			switch {
			case ctx.Err() != nil:
				summary.Interrupted++
			case err != nil:
				summary.Failed++
			default:
				summary.Passed++
			}
			if result != nil {
				result(Event{Cycle: cycle, Attack: step.Attack, Elapsed: time.Since(start), Err: err, Summary: summary})
			}
			if ctx.Err() != nil {
				return summary, ctx.Err()
			}
			if err != nil {
				return summary, fmt.Errorf("%s failed; continuous run stopped: %w", step.Attack, err)
			}
			if i == len(plan)-1 {
				summary.Cycles = cycle
			}
			if cycles > 0 && cycle == cycles && i == len(plan)-1 {
				return summary, nil
			}
			timer := time.NewTimer(gap)
			select {
			case <-ctx.Done():
				timer.Stop()
				return summary, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return summary, nil
}
