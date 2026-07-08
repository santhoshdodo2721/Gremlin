package plugins

import (
"context"
"fmt"
"strconv"
"time"

"gremlin-in-a-box/internal/network"
)

type LatencyPlugin struct {
net *network.Manager
}

func NewLatencyPlugin(net *network.Manager) *LatencyPlugin {
return &LatencyPlugin{net: net}
}

func (p *LatencyPlugin) Name() string { return "latency" }

func (p *LatencyPlugin) Describe() string {
return "Injects network latency into a container using tc/netem (params: delay_ms, jitter_ms, duration_s)"
}

func (p *LatencyPlugin) Run(ctx context.Context, target string, params Params) (Result, error) {
delayMs := atoiOr(params["delay_ms"], 200)
jitterMs := atoiOr(params["jitter_ms"], 20)
durationS := atoiOr(params["duration_s"], 30)

if _, err := p.net.AddLatency(ctx, target, delayMs, jitterMs); err != nil {
return Result{Success: false, Message: err.Error()}, err
}

select {
case <-time.After(time.Duration(durationS) * time.Second):
case <-ctx.Done():
}

if _, err := p.net.Clear(ctx, target); err != nil {
return Result{Success: false, Message: fmt.Sprintf("attack ran but cleanup failed: %v", err)}, err
}

return Result{
Success: true,
Message: fmt.Sprintf("injected %dms (+/-%dms) latency for %ds", delayMs, jitterMs, durationS),
Metadata: Params{
"delay_ms":   strconv.Itoa(delayMs),
"jitter_ms":  strconv.Itoa(jitterMs),
"duration_s": strconv.Itoa(durationS),
},
}, nil
}

func atoiOr(s string, fallback int) int {
if s == "" {
return fallback
}
n, err := strconv.Atoi(s)
if err != nil {
return fallback
}
return n
}
