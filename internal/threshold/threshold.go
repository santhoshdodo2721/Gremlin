package threshold

import (
"context"
"fmt"
"time"

"gremlin-in-a-box/internal/dockerapi"
"gremlin-in-a-box/internal/health"
"gremlin-in-a-box/internal/network"
)

// Result is the outcome of one threshold-finding run: the highest attack
// intensity the target tolerated before its health check degraded past
// the failure line.
type Result struct {
Attack        string `json:"attack"`
Target        string `json:"target"`
BreakingPoint int    `json:"breaking_point"`
Unit          string `json:"unit"`
TimestampUnix int64  `json:"timestamp_unix"`
}

// checkStepHealthy measures health once, and if it looks unhealthy, waits
// and measures again before accepting that as a real failure - this
// protects against a single noisy ping flipping the whole test result.
func checkStepHealthy(ctx context.Context, healthURL string, failureLine int64) (avgMs int64, ok bool) {
avgMs, healthy := health.AverageLatency(ctx, healthURL, 8)
if healthy && avgMs >= 0 && avgMs <= failureLine {
return avgMs, true
}

// First measurement looked bad - confirm with a second attempt before
// trusting it, since a single noisy sample is not a real finding.
time.Sleep(1 * time.Second)
avgMs2, healthy2 := health.AverageLatency(ctx, healthURL, 8)
if healthy2 && avgMs2 >= 0 && avgMs2 <= failureLine {
return avgMs2, true
}

return avgMs2, false
}

// measureBaseline gets a stable baseline, retrying once if the first
// attempt looks unhealthy - avoids a cold-start or leftover state from a
// previous test poisoning the very first measurement.
func measureBaseline(ctx context.Context, healthURL string) (int64, error) {
baselineMs, healthy := health.AverageLatency(ctx, healthURL, 8)
if healthy && baselineMs >= 0 {
return baselineMs, nil
}
time.Sleep(2 * time.Second)
baselineMs, healthy = health.AverageLatency(ctx, healthURL, 8)
if !healthy || baselineMs < 0 {
return 0, fmt.Errorf("target unhealthy before test even started")
}
return baselineMs, nil
}

// RunLatencyThreshold escalates network latency step by step until the
// target health check either fails outright (confirmed twice) or its
// response time exceeds 3x its unattacked baseline. It returns the last
// step that was still healthy.
func RunLatencyThreshold(ctx context.Context, net *network.Manager, target, healthURL string) (Result, error) {
time.Sleep(3 * time.Second)
	baselineMs, err := measureBaseline(ctx, healthURL)
if err != nil {
return Result{}, err
}
failureLine := baselineMs * 3
if failureLine < 200 {
failureLine = 500
}

steps := []int{100, 200, 400, 800, 1600}
lastGood := 0

for _, delayMs := range steps {
if _, err := net.AddLatency(ctx, target, delayMs, delayMs/10); err != nil {
net.Clear(ctx, target)
break
}
time.Sleep(1 * time.Second)

_, ok := checkStepHealthy(ctx, healthURL, failureLine)
net.Clear(ctx, target)
time.Sleep(2 * time.Second)

if !ok {
break
}
lastGood = delayMs
}

return Result{
Attack:        "latency",
Target:        target,
BreakingPoint: lastGood,
Unit:          "ms",
TimestampUnix: time.Now().Unix(),
}, nil
}

// RunCPUThreshold escalates stress-ng worker count until the health check
// degrades past 3x baseline (confirmed twice) or fails outright.
func RunCPUThreshold(ctx context.Context, docker *dockerapi.Client, target, healthURL string) (Result, error) {
time.Sleep(3 * time.Second)
	baselineMs, err := measureBaseline(ctx, healthURL)
if err != nil {
return Result{}, err
}
failureLine := baselineMs * 3
if failureLine < 200 {
failureLine = 500
}

steps := []int{1, 2, 4, 8}
lastGood := 0

for _, workers := range steps {
cmd := []string{"stress-ng", "--cpu", fmt.Sprintf("%d", workers), "--timeout", "8s"}
if err := docker.StartBackground(ctx, target, cmd); err != nil {
break
}
time.Sleep(2 * time.Second)

_, ok := checkStepHealthy(ctx, healthURL, failureLine)

time.Sleep(6 * time.Second)

if !ok {
break
}
lastGood = workers
}

return Result{
Attack:        "cpu",
Target:        target,
BreakingPoint: lastGood,
Unit:          "workers",
TimestampUnix: time.Now().Unix(),
}, nil
}
