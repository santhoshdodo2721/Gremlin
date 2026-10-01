package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gremlin-in-a-box/internal/config"
	"gremlin-in-a-box/internal/continuous"
	"gremlin-in-a-box/internal/controller"
	"gremlin-in-a-box/internal/dockerapi"
	"gremlin-in-a-box/internal/plugins"
	"gremlin-in-a-box/internal/tui"
)

func continuousPlan(names []string, duration int) ([]continuous.Step, error) {
	if duration < 1 || duration > 3600 {
		return nil, fmt.Errorf("duration must be between 1 and 3600 seconds")
	}
	var plan []continuous.Step
	for _, name := range names {
		name = strings.TrimSpace(name)
		params := plugins.Params{"duration_s": strconv.Itoa(duration)}
		switch name {
		case "latency":
			params["delay_ms"] = "150"
			params["jitter_ms"] = "15"
		case "packet-loss":
			params["percent"] = "5"
		case "corruption":
			params["percent"] = "1"
		case "cpu":
			params["method"] = "throttle"
			params["quota_pct"] = "30"
		case "container-pause":
		case "container-kill":
			params["restart"] = "true"
		default:
			return nil, fmt.Errorf("unsupported continuous attack %q", name)
		}
		plan = append(plan, continuous.Step{Attack: name, Params: params})
	}
	if len(plan) == 0 {
		return nil, fmt.Errorf("choose at least one attack")
	}
	return plan, nil
}

func applicationTarget(ctx context.Context, docker *dockerapi.Client, cfg config.Config, target string) (dockerapi.Container, error) {
	if strings.TrimSpace(target) == "" {
		return dockerapi.Container{}, fmt.Errorf("choose an application container with --target")
	}
	containers, err := docker.ListApplicationContainers(ctx, cfg.ApplicationDirectory)
	if err != nil {
		return dockerapi.Container{}, err
	}
	for _, container := range containers {
		if container.Matches(target) {
			return container, nil
		}
	}
	return dockerapi.Container{}, fmt.Errorf("%q is not a running service in this application", target)
}

func executeContinuous(docker *dockerapi.Client, ctrl *controller.Controller, cfg config.Config, target, healthURL string, plan []continuous.Step, gap time.Duration, cycles int) error {
	if healthURL == "" {
		return fmt.Errorf("continuous tests require a recovery endpoint")
	}
	selected, err := applicationTarget(context.Background(), docker, cfg, target)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	quiet := *ctrl
	if os.Getenv("GREMLIN_DEBUG") != "true" {
		quiet.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	tui.Card("CONTINUOUS RUN", []string{
		"Service    " + selected.DisplayName(),
		"Recovery   " + healthURL,
		fmt.Sprintf("Tests      %d per cycle / %s between tests", len(plan), gap),
		"Control    Ctrl+C to stop and restore the active fault",
	})
	fmt.Printf("  %-7s %-19s %-10s %-10s %s\n", "CYCLE", "FAULT", "TIME", "RESULT", "DETAIL")
	run := func(ctx context.Context, name, target string, params plugins.Params, url string) error {
		// Recheck membership and keep the original container ID throughout the run.
		if _, err := applicationTarget(ctx, docker, cfg, selected.ID); err != nil {
			return err
		}
		return quiet.RunAttackWithHealthURL(ctx, name, selected.ID, params, url)
	}
	summary, err := continuous.Run(ctx, run, selected.ID, plan, healthURL, gap, cycles, func(event continuous.Event) {
		status, color, detail := "PASS", tui.Green, "Recovery verified"
		if event.Err != nil {
			status, color, detail = "FAIL", tui.Red, event.Err.Error()
		}
		if ctx.Err() != nil {
			status, color, detail = "STOP", tui.Yellow, "Interrupted; cleanup completed or attempted (see report)"
		}
		fmt.Printf("  %-7d %-19s %-10s %s%-10s%s %s\n", event.Cycle, event.Attack, event.Elapsed.Round(time.Millisecond), color, status, tui.Reset, detail)
	})
	fmt.Printf("\n  Completed cycles: %d   Passed: %d   Failed: %d   Interrupted: %d\n", summary.Cycles, summary.Passed, summary.Failed, summary.Interrupted)
	fmt.Println("  Results saved to " + cfg.ReportsFile)
	if errors.Is(err, context.Canceled) {
		tui.Success("Continuous run stopped.")
		return nil
	}
	return err
}

func continuousMenu(docker *dockerapi.Client, ctrl *controller.Controller, cfg config.Config) {
	tui.Clear()
	tui.Banner()
	target, url := selectTargetAndHealth(docker, cfg)
	if target == "" {
		return
	}
	if url == "" {
		tui.Warning("A recovery endpoint is required for continuous tests.")
		tui.Pause()
		return
	}
	choice := tui.Menu("Continuous test profile", []string{
		"Complete cycle       All six failure types",
		"Network cycle        Latency, packet loss, corruption",
		"CPU throttling       Repeat a CPU capacity test",
		"Service freeze       Repeat a pause and recovery test",
	})
	if choice < 0 {
		return
	}
	profiles := [][]string{{"latency", "packet-loss", "corruption", "cpu", "container-pause", "container-kill"}, {"latency", "packet-loss", "corruption"}, {"cpu"}, {"container-pause"}}
	duration, err := strconv.Atoi(tui.AskDefault("Fault duration in seconds", "5"))
	if err != nil {
		tui.Warning("Enter a whole number of seconds.")
		tui.Pause()
		return
	}
	interval, err := strconv.Atoi(tui.AskDefault("Wait between tests in seconds", "10"))
	if err != nil || interval < 1 || interval > 86400 {
		tui.Warning("Interval must be between 1 and 86400 seconds.")
		tui.Pause()
		return
	}
	plan, err := continuousPlan(profiles[choice], duration)
	if err != nil {
		tui.Warning(err.Error())
		tui.Pause()
		return
	}
	tui.Info("Repeats until stopped. A failed test ends the run for inspection.")
	if !tui.AskConfirm("Start continuous testing of "+target+"?", false) {
		return
	}
	if err := executeContinuous(docker, ctrl, cfg, target, url, plan, time.Duration(interval)*time.Second, 0); err != nil {
		tui.Warning(err.Error())
	}
	tui.Pause()
}

func runContinuousCmd(docker *dockerapi.Client, ctrl *controller.Controller, cfg config.Config) {
	fs := flag.NewFlagSet("continuous", flag.ExitOnError)
	target := fs.String("target", "", "application container name or ID")
	healthURL := fs.String("health-url", cfg.TargetHealthURL, "recovery endpoint (required)")
	names := fs.String("attacks", "latency,cpu,container-pause", "comma-separated failure types")
	duration := fs.Int("duration-s", 5, "duration of each fault, in seconds")
	interval := fs.Int("interval-s", 10, "quiet period after each recovered test, in seconds")
	cycles := fs.Int("cycles", 0, "number of cycles; 0 repeats until Ctrl+C")
	fs.Parse(os.Args[2:])
	plan, err := continuousPlan(strings.Split(*names, ","), *duration)
	if err == nil && (*interval < 1 || *interval > 86400 || *cycles < 0) {
		err = fmt.Errorf("interval must be 1–86400 seconds; cycles cannot be negative")
	}
	if err == nil {
		err = executeContinuous(docker, ctrl, cfg, *target, *healthURL, plan, time.Duration(*interval)*time.Second, *cycles)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "continuous run error:", err)
		os.Exit(1)
	}
}
