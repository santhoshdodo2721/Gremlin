package plugins

import (
	"context"
	"fmt"
	"time"

	"gremlin-in-a-box/internal/dockerapi"
)

type ContainerPausePlugin struct {
	docker *dockerapi.Client
}

func NewContainerPausePlugin(docker *dockerapi.Client) *ContainerPausePlugin {
	return &ContainerPausePlugin{docker: docker}
}

func (p *ContainerPausePlugin) Name() string { return "container-pause" }

func (p *ContainerPausePlugin) Describe() string {
	return "Freezes/pauses a container to simulate a hung or unresponsive service (params: duration_s)"
}

func (p *ContainerPausePlugin) Run(ctx context.Context, target string, params Params) (Result, error) {
	durationS := atoiOr(params["duration_s"], 15)

	if err := p.docker.PauseContainer(ctx, target); err != nil {
		return Result{Success: false, Message: fmt.Sprintf("pause failed: %v", err)}, err
	}

	defer func() {
		p.docker.UnpauseContainer(context.Background(), target)
	}()

	select {
	case <-time.After(time.Duration(durationS) * time.Second):
	case <-ctx.Done():
	}

	if err := p.docker.UnpauseContainer(context.Background(), target); err != nil {
		return Result{Success: false, Message: fmt.Sprintf("unpause failed: %v", err)}, err
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("paused and unpaused container %s after %ds", target, durationS),
		Metadata: Params{
			"duration_s": fmt.Sprintf("%d", durationS),
		},
	}, nil
}
