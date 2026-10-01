package plugins

import (
	"context"
	"fmt"
	"strconv"
	"time"

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
	return "Exhausts or throttles CPU (auto-falls back to Cgroup throttling for any container; params: quota_pct, workers, duration_s, method)"
}

func (p *CPUPlugin) Run(ctx context.Context, target string, params Params) (Result, error) {
	method := params["method"]
	durationS := atoiOr(params["duration_s"], 15)
	quotaPct := atoiOr(params["quota_pct"], 10)
	workers := atoiOr(params["workers"], 2)

	if method == "throttle" {
		return p.runCgroupThrottle(ctx, target, quotaPct, durationS)
	}

	if method == "stress" {
		cmd := []string{"stress-ng", "--cpu", strconv.Itoa(workers), "--timeout", fmt.Sprintf("%ds", durationS)}
		out, err := p.docker.Exec(ctx, target, cmd)
		if err != nil {
			return Result{Success: false, Message: err.Error()}, err
		}
		return Result{
			Success:  true,
			Message:  fmt.Sprintf("ran %d cpu workers for %ds (stress-ng)", workers, durationS),
			Metadata: Params{"workers": strconv.Itoa(workers), "duration_s": strconv.Itoa(durationS), "output": out},
		}, nil
	}

	// Default "auto": try stress-ng if available; if missing, fall back to cgroup throttling!
	cmd := []string{"stress-ng", "--cpu", strconv.Itoa(workers), "--timeout", fmt.Sprintf("%ds", durationS)}
	out, err := p.docker.Exec(ctx, target, cmd)
	if err == nil {
		return Result{
			Success:  true,
			Message:  fmt.Sprintf("ran %d cpu workers for %ds (stress-ng)", workers, durationS),
			Metadata: Params{"workers": strconv.Itoa(workers), "duration_s": strconv.Itoa(durationS), "output": out},
		}, nil
	}

	return p.runCgroupThrottle(ctx, target, quotaPct, durationS)
}

func (p *CPUPlugin) runCgroupThrottle(ctx context.Context, target string, quotaPct, durationS int) (Result, error) {
	info, err := p.docker.InspectContainer(ctx, target)
	if err != nil {
		return Result{Success: false, Message: fmt.Sprintf("inspect container: %v", err)}, err
	}

	origPeriod := 100000
	origQuota := -1
	if hostConfig, ok := info["HostConfig"].(map[string]any); ok {
		if period, ok := hostConfig["CpuPeriod"].(float64); ok && period > 0 {
			origPeriod = int(period)
		}
		if quota, ok := hostConfig["CpuQuota"].(float64); ok && quota > 0 {
			origQuota = int(quota)
		}
	}

	period := 100000
	quota := (period * quotaPct) / 100
	if quota <= 0 {
		quota = 5000 // min 5%
	}

	if err := p.docker.UpdateContainer(ctx, target, map[string]any{
		"CpuPeriod": period,
		"CpuQuota":  quota,
	}); err != nil {
		return Result{Success: false, Message: fmt.Sprintf("cgroup cpu throttle failed: %v", err)}, err
	}

	select {
	case <-time.After(time.Duration(durationS) * time.Second):
	case <-ctx.Done():
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := p.docker.UpdateContainer(cleanupCtx, target, map[string]any{
		"CpuPeriod": origPeriod, "CpuQuota": origQuota,
	}); err != nil {
		return Result{Success: false}, fmt.Errorf("restore CPU limits: %w", err)
	}
	if ctx.Err() != nil {
		return Result{Success: false}, ctx.Err()
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("throttled container CPU to %d%% quota (%d/%d) for %ds", quotaPct, quota, period, durationS),
		Metadata: Params{
			"method":     "cgroup_throttle",
			"quota_pct":  strconv.Itoa(quotaPct),
			"duration_s": strconv.Itoa(durationS),
		},
	}, nil
}
