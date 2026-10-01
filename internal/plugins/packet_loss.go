package plugins

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"gremlin-in-a-box/internal/network"
)

type PacketLossPlugin struct {
	net *network.Manager
}

func NewPacketLossPlugin(net *network.Manager) *PacketLossPlugin {
	return &PacketLossPlugin{net: net}
}

func (p *PacketLossPlugin) Name() string { return "packet-loss" }

func (p *PacketLossPlugin) Describe() string {
	return "Injects network packet loss into a container (params: percent, duration_s)"
}

func (p *PacketLossPlugin) Run(ctx context.Context, target string, params Params) (Result, error) {
	percentStr := params["percent"]
	if percentStr == "" {
		percentStr = "20"
	}
	percent, err := strconv.ParseFloat(percentStr, 64)
	if err != nil {
		percent = 20.0
	}
	durationS := atoiOr(params["duration_s"], 15)

	if _, err := p.net.AddPacketLoss(ctx, target, percent); err != nil {
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
		Message: fmt.Sprintf("injected %.1f%% packet loss for %ds", percent, durationS),
		Metadata: Params{
			"percent":    fmt.Sprintf("%.1f", percent),
			"duration_s": strconv.Itoa(durationS),
		},
	}, nil
}
