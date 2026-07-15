package loadtest

import (
"context"
"fmt"
"net/http"
"sync"
"sync/atomic"
"time"

"gremlin-in-a-box/internal/dockerapi"
)

// Result is the outcome of one load test: how many concurrent virtual
// users the target tolerated before its error rate or latency crossed the
// failure line, plus what resources it was actually using at that point.
type Result struct {
MaxConcurrentUsers int     `json:"max_concurrent_users"`
RequestsPerSecond  float64 `json:"requests_per_second_at_max"`
AvgLatencyMs       int64   `json:"avg_latency_ms_at_max"`
ErrorRatePercent   float64 `json:"error_rate_percent_at_max"`
CPUPercent         float64 `json:"cpu_percent_at_max"`
MemUsedMB          float64 `json:"mem_used_mb_at_max"`
TimestampUnix      int64   `json:"timestamp_unix"`
}

// stepResult is what one concurrency level produced.
type stepResult struct {
concurrency  int
totalReqs    int64
totalErrors  int64
totalLatency int64
cpuPercent   float64
memUsedMB    float64
}

// runStep fires `concurrency` virtual users continuously against url for
// duration, each firing requests back-to-back, and returns aggregate stats
// plus one resource snapshot taken partway through the run.
func runStep(ctx context.Context, docker *dockerapi.Client, containerID, url string, concurrency int, duration time.Duration) stepResult {
var totalReqs, totalErrors, totalLatency int64
var wg sync.WaitGroup

stepCtx, cancel := context.WithTimeout(ctx, duration)
defer cancel()

client := http.Client{Timeout: 5 * time.Second}

for i := 0; i < concurrency; i++ {
wg.Add(1)
go func() {
defer wg.Done()
for {
select {
case <-stepCtx.Done():
return
default:
}
start := time.Now()
req, err := http.NewRequestWithContext(stepCtx, http.MethodGet, url, nil)
if err != nil {
return
}
resp, err := client.Do(req)
elapsed := time.Since(start).Milliseconds()
atomic.AddInt64(&totalReqs, 1)
if err != nil || resp.StatusCode != http.StatusOK {
atomic.AddInt64(&totalErrors, 1)
} else {
atomic.AddInt64(&totalLatency, elapsed)
}
if resp != nil {
resp.Body.Close()
}
}
}()
}

// Sample real resource usage roughly halfway through the load window.
var cpuPercent, memUsedMB float64
go func() {
time.Sleep(duration / 2)
if stats, err := docker.GetStats(ctx, containerID); err == nil {
cpuPercent = stats.CPUPercent
memUsedMB = stats.MemUsedMB
}
}()

wg.Wait()

return stepResult{
concurrency:  concurrency,
totalReqs:    totalReqs,
totalErrors:  totalErrors,
totalLatency: totalLatency,
cpuPercent:   cpuPercent,
memUsedMB:    memUsedMB,
}
}

// Run escalates concurrent virtual users - 5, 10, 25, 50, 100, 200 - each
// hammering url for a few seconds, until the error rate exceeds 5%% or
// average latency exceeds 1 second. It returns the last concurrency level
// that stayed healthy, along with real CPU/memory usage measured at that
// level.
func Run(ctx context.Context, docker *dockerapi.Client, containerID, url string) (Result, error) {
levels := []int{5, 10, 25, 50, 100, 200}
stepDuration := 5 * time.Second

var lastGood stepResult
found := false

for _, level := range levels {
step := runStep(ctx, docker, containerID, url, level, stepDuration)

errorRate := 0.0
if step.totalReqs > 0 {
errorRate = 100.0 * float64(step.totalErrors) / float64(step.totalReqs)
}
successCount := step.totalReqs - step.totalErrors
avgLatency := int64(0)
if successCount > 0 {
avgLatency = step.totalLatency / successCount
}

if errorRate > 5.0 || avgLatency > 1000 {
break
}

lastGood = step
found = true

// Brief cooldown between levels so one step's load does not
// bleed into the next step's measurement.
time.Sleep(2 * time.Second)
}

if !found {
return Result{}, fmt.Errorf("target failed even at the lowest concurrency level tested")
}

rps := float64(lastGood.totalReqs) / stepDuration.Seconds()
errorRate := 0.0
if lastGood.totalReqs > 0 {
errorRate = 100.0 * float64(lastGood.totalErrors) / float64(lastGood.totalReqs)
}
successCount := lastGood.totalReqs - lastGood.totalErrors
avgLatency := int64(0)
if successCount > 0 {
avgLatency = lastGood.totalLatency / successCount
}

return Result{
MaxConcurrentUsers: lastGood.concurrency,
RequestsPerSecond:  rps,
AvgLatencyMs:       avgLatency,
ErrorRatePercent:   errorRate,
CPUPercent:         lastGood.cpuPercent,
MemUsedMB:          lastGood.memUsedMB,
TimestampUnix:      time.Now().Unix(),
}, nil
}
