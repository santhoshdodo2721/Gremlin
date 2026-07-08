package scheduler

import (
"context"
"log/slog"
"time"

"gremlin-in-a-box/internal/controller"
"gremlin-in-a-box/internal/plugins"
)

type Job struct {
Attack string
Target string
Params plugins.Params
Every  time.Duration
}

func Run(ctx context.Context, log *slog.Logger, ctrl *controller.Controller, jobs []Job) {
for _, job := range jobs {
go runJob(ctx, log, ctrl, job)
}
<-ctx.Done()
}

func runJob(ctx context.Context, log *slog.Logger, ctrl *controller.Controller, job Job) {
ticker := time.NewTicker(job.Every)
defer ticker.Stop()

for {
select {
case <-ctx.Done():
return
case <-ticker.C:
log.Info("scheduled attack firing", "attack", job.Attack, "target", job.Target)
if err := ctrl.RunAttack(ctx, job.Attack, job.Target, job.Params); err != nil {
log.Error("scheduled attack failed", "attack", job.Attack, "error", err)
}
}
}
}
