package controller

import (
"context"
"fmt"
"log/slog"
"net/http"
"time"

"gremlin-in-a-box/internal/plugins"
"gremlin-in-a-box/internal/reports"
)

type Controller struct {
Log             *slog.Logger
Reports         *reports.Store
TargetHealthURL string
}

func New(log *slog.Logger, r *reports.Store, healthURL string) *Controller {
return &Controller{Log: log, Reports: r, TargetHealthURL: healthURL}
}

func (c *Controller) RunAttack(ctx context.Context, attackName, target string, params plugins.Params) error {
plugin, ok := plugins.Get(attackName)
if !ok {
return fmt.Errorf("unknown attack %q (run gremlin status to list available attacks)", attackName)
}

c.Log.Info("starting attack", "attack", attackName, "target", target)
start := time.Now()

result, runErr := plugin.Run(ctx, target, params)
durationMs := time.Since(start).Milliseconds()

recoveryMs := c.measureRecovery(ctx)

success := runErr == nil && result.Success

report := reports.Report{
ID:         fmt.Sprintf("%s-%d", attackName, start.Unix()),
Attack:     attackName,
Target:     target,
StartedAt:  start,
DurationMs: durationMs,
RecoveryMs: recoveryMs,
Success:    success,
Message:    result.Message,
}
if runErr != nil {
report.Message = runErr.Error()
}
if err := c.Reports.Save(report); err != nil {
c.Log.Error("failed to save report", "error", err)
}

c.Log.Info("attack finished", "attack", attackName, "success", success,
"duration_ms", durationMs, "recovery_ms", recoveryMs)

return runErr
}

func (c *Controller) measureRecovery(ctx context.Context) int64 {
if c.TargetHealthURL == "" {
return 0
}
start := time.Now()
deadline := start.Add(60 * time.Second)
client := http.Client{Timeout: 2 * time.Second}

for time.Now().Before(deadline) {
select {
case <-ctx.Done():
return -1
default:
}
resp, err := client.Get(c.TargetHealthURL)
if err == nil && resp.StatusCode == http.StatusOK {
resp.Body.Close()
return time.Since(start).Milliseconds()
}
if resp != nil {
resp.Body.Close()
}
time.Sleep(500 * time.Millisecond)
}
return -1
}
