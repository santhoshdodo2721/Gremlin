package continuous

import (
	"context"
	"errors"
	"gremlin-in-a-box/internal/plugins"
	"reflect"
	"testing"
	"time"
)

func TestSequentialCycles(t *testing.T) {
	var calls []string
	var active bool
	run := func(ctx context.Context, name, target string, params plugins.Params, url string) error {
		if active {
			t.Fatal("overlapping faults")
		}
		active = true
		defer func() { active = false }()
		calls = append(calls, name)
		if target != "api" || url != "health" {
			t.Fatal("lost target or health endpoint")
		}
		return nil
	}
	summary, err := Run(context.Background(), run, "api", []Step{{Attack: "latency"}, {Attack: "cpu"}}, "health", time.Millisecond, 2, nil)
	if err != nil || summary.Passed != 4 || summary.Cycles != 2 || !reflect.DeepEqual(calls, []string{"latency", "cpu", "latency", "cpu"}) {
		t.Fatalf("summary=%+v calls=%v error=%v", summary, calls, err)
	}
}
func TestFailureStopsBeforeAnotherFault(t *testing.T) {
	calls := 0
	run := func(context.Context, string, string, plugins.Params, string) error {
		calls++
		return errors.New("cleanup failed")
	}
	summary, err := Run(context.Background(), run, "api", []Step{{Attack: "cpu"}, {Attack: "latency"}}, "health", time.Millisecond, 0, nil)
	if err == nil || calls != 1 || summary.Failed != 1 {
		t.Fatalf("calls=%d summary=%+v err=%v", calls, summary, err)
	}
}
func TestCancellationWaitsForCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cleaned := false
	run := func(ctx context.Context, _ string, _ string, _ plugins.Params, _ string) error {
		cancel()
		<-ctx.Done()
		cleaned = true
		return ctx.Err()
	}
	summary, err := Run(ctx, run, "api", []Step{{Attack: "cpu"}}, "health", time.Hour, 0, nil)
	if !cleaned || !errors.Is(err, context.Canceled) || summary.Interrupted != 1 {
		t.Fatalf("cleanup=%v summary=%+v err=%v", cleaned, summary, err)
	}
}
func TestStopDuringQuietPeriod(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	run := func(context.Context, string, string, plugins.Params, string) error { calls++; return nil }
	start := time.Now()
	_, err := Run(ctx, run, "api", []Step{{Attack: "cpu"}}, "health", time.Hour, 0, func(Event) { cancel() })
	if calls != 1 || !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
