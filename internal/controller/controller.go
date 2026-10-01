package controller

import (
	"context"
	"errors"
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
	return c.RunAttackWithHealthURL(ctx, attackName, target, params, c.TargetHealthURL)
}

func (c *Controller) RunAttackWithHealthURL(ctx context.Context, attackName, target string, params plugins.Params, healthURL string) error {
	plugin, ok := plugins.Get(attackName)
	if !ok {
		return fmt.Errorf("unknown attack %q (run gremlin status to list available attacks)", attackName)
	}

	if err := plugins.ValidateParams(attackName, target, params); err != nil {
		return err
	}

	c.Log.Info("starting attack", "attack", attackName, "target", target, "health_url", healthURL)
	start := time.Now()

	result, runErr := plugin.Run(ctx, target, params)
	durationMs := time.Since(start).Milliseconds()

	recoveryMs := c.measureRecovery(ctx, healthURL)

	if runErr == nil && !result.Success {
		runErr = fmt.Errorf("attack did not succeed: %s", result.Message)
	}
	if runErr == nil && recoveryMs < 0 {
		runErr = fmt.Errorf("service recovery could not be verified at %s", healthURL)
	}
	if runErr == nil && ctx.Err() != nil {
		runErr = ctx.Err()
	}
	success := runErr == nil
	recoveryStatus := "verified"
	if healthURL == "" {
		recoveryStatus = "skipped"
	} else if recoveryMs < 0 {
		recoveryStatus = "unverified"
	}

	report := reports.Report{
		ID:             fmt.Sprintf("%s-%d", attackName, start.UnixNano()),
		Attack:         attackName,
		Target:         target,
		StartedAt:      start,
		DurationMs:     durationMs,
		RecoveryMs:     recoveryMs,
		RecoveryStatus: recoveryStatus,
		Success:        success,
		Message:        result.Message,
	}
	if runErr != nil {
		report.Message = runErr.Error()
	}
	if err := c.Reports.Save(report); err != nil {
		c.Log.Error("failed to save report", "error", err)
		runErr = errors.Join(runErr, fmt.Errorf("save report: %w", err))
	}

	c.Log.Info("attack finished", "attack", attackName, "success", success,
		"duration_ms", durationMs, "recovery_ms", recoveryMs)

	return runErr
}

func (c *Controller) measureRecovery(ctx context.Context, healthURL string) int64 {
	if healthURL == "" {
		return 0
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	client := http.Client{Timeout: 2 * time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return -1
		}
		resp, err := client.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		if err == nil && resp.StatusCode == http.StatusOK {
			return time.Since(start).Milliseconds()
		}
		select {
		case <-ctx.Done():
			return -1
		case <-time.After(500 * time.Millisecond):
		}
	}
}
