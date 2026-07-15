package health

import (
"context"
"net/http"
"time"
)

func Ping(ctx context.Context, url string) (healthy bool, latencyMs int64) {
client := http.Client{Timeout: 5 * time.Second}
req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
if err != nil {
return false, -1
}
start := time.Now()
resp, err := client.Do(req)
elapsed := time.Since(start).Milliseconds()
if err != nil {
return false, -1
}
defer resp.Body.Close()
return resp.StatusCode == http.StatusOK, elapsed
}

// AverageLatency pings n times and returns the average latency across
// successful pings, plus whether at least 80%% of pings succeeded - a
// majority-based health signal instead of an all-or-nothing one, so a
// single noisy ping does not flip the whole result.
func AverageLatency(ctx context.Context, url string, n int) (avgMs int64, healthy bool) {
var total int64
successes := 0
for i := 0; i < n; i++ {
ok, ms := Ping(ctx, url)
if ok {
total += ms
successes++
}
time.Sleep(200 * time.Millisecond)
}
if successes == 0 {
return -1, false
}
successRate := float64(successes) / float64(n)
return total / int64(successes), successRate >= 0.8
}
