package plugins

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"gremlin-in-a-box/internal/network"
)

type CorruptionPlugin struct {
	net *network.Manager
}

func NewCorruptionPlugin(net *network.Manager) *CorruptionPlugin {
	return &CorruptionPlugin{net: net}
}

func (p *CorruptionPlugin) Name() string { return "corruption" }

func (p *CorruptionPlugin) Describe() string {
	return "Corrupts network packets for a container using tc/netem (params: percent, duration_s)"
}

func (p *CorruptionPlugin) Run(ctx context.Context, target string, params Params) (Result, error) {
	percentStr := params["percent"]
	if percentStr == "" {
		percentStr = "10"
	}
	percent, err := strconv.ParseFloat(percentStr, 64)
	if err != nil {
		percent = 10.0
	}
	durationS := atoiOr(params["duration_s"], 15)

	if _, err := p.net.AddCorruption(ctx, target, percent); err != nil {
		return Result{Success: false, Message: err.Error()}, err
	}

	select {
	case <-time.After(time.Duration(durationS) * time.Second):
	case <-ctx.Done():
	}

	if _, err := p.net.Clear(context.Background(), target); err != nil {
		return Result{Success: false, Message: fmt.Sprintf("attack ran but cleanup failed: %v", err)}, err
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("injected %.1f%% packet corruption for %ds", percent, durationS),
		Metadata: Params{
			"percent":    fmt.Sprintf("%.1f", percent),
			"duration_s": strconv.Itoa(durationS),
		},
	}, nil
}
