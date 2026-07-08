package plugins

import (
"context"
"fmt"

"gremlin-in-a-box/internal/dockerapi"
)

type ContainerKillPlugin struct {
docker *dockerapi.Client
}

func NewContainerKillPlugin(docker *dockerapi.Client) *ContainerKillPlugin {
return &ContainerKillPlugin{docker: docker}
}

func (p *ContainerKillPlugin) Name() string { return "container-kill" }

func (p *ContainerKillPlugin) Describe() string {
return "Stops a container to simulate a crash (params: restart, stop_timeout_s)"
}

func (p *ContainerKillPlugin) Run(ctx context.Context, target string, params Params) (Result, error) {
stopTimeout := atoiOr(params["stop_timeout_s"], 5)

if err := p.docker.StopContainer(ctx, target, stopTimeout); err != nil {
return Result{Success: false, Message: err.Error()}, err
}

shouldRestart := params["restart"] != "false"
if !shouldRestart {
return Result{
Success: true,
Message: fmt.Sprintf("stopped container %s (left down for recovery testing)", target),
}, nil
}

if err := p.docker.RestartContainer(ctx, target, stopTimeout); err != nil {
return Result{Success: false, Message: fmt.Sprintf("stopped but failed to restart: %v", err)}, err
}

return Result{
Success: true,
Message: fmt.Sprintf("killed and restarted container %s", target),
}, nil
}
