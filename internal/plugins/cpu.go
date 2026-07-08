package plugins

import (
"context"
"fmt"
"strconv"

"gremlin-in-a-box/internal/dockerapi"
)

type CPUPlugin struct {
docker *dockerapi.Client
}

func NewCPUPlugin(docker *dockerapi.Client) *CPUPlugin {
return &CPUPlugin{docker: docker}
}

func (p *CPUPlugin) Name() string { return "cpu" }

func (p *CPUPlugin) Describe() string {
return "Exhausts CPU inside a container using stress-ng (params: workers, duration_s)"
}

func (p *CPUPlugin) Run(ctx context.Context, target string, params Params) (Result, error) {
workers := atoiOr(params["workers"], 2)
durationS := atoiOr(params["duration_s"], 30)

cmd := []string{"stress-ng", "--cpu", strconv.Itoa(workers), "--timeout", fmt.Sprintf("%ds", durationS)}

out, err := p.docker.Exec(ctx, target, cmd)
if err != nil {
return Result{Success: false, Message: err.Error()}, err
}

return Result{
Success: true,
Message: fmt.Sprintf("ran %d cpu workers for %ds", workers, durationS),
Metadata: Params{
"workers":    strconv.Itoa(workers),
"duration_s": strconv.Itoa(durationS),
"output":     out,
},
}, nil
}
