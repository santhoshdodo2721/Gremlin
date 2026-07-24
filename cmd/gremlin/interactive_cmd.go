package main

import (
	"context"
	"fmt"
	"time"

	"gremlin-in-a-box/internal/config"
	"gremlin-in-a-box/internal/controller"
	"gremlin-in-a-box/internal/dockerapi"
	"gremlin-in-a-box/internal/loadtest"
	"gremlin-in-a-box/internal/metrics"
	"gremlin-in-a-box/internal/network"
	"gremlin-in-a-box/internal/plugins"
	"gremlin-in-a-box/internal/reports"
	"gremlin-in-a-box/internal/resilience"
	"gremlin-in-a-box/internal/threshold"
	"gremlin-in-a-box/internal/tui"
)

func runInteractiveMenu(cfg config.Config, docker *dockerapi.Client, ctrl *controller.Controller, store *reports.Store, m *metrics.Collector) {
	for {
		tui.Clear()
		tui.Banner()

		choice := tui.Menu("Main Menu", []string{
			"Run an attack (container-kill / latency / cpu)",
			"Find resilience breaking point (auto-escalating threshold test)",
			"Run a load test (find max concurrent users)",
			"View attack report history",
			"View resilience history",
			"Show registered attack plugins",
			"Start metrics server (blocks - Ctrl+C to stop)",
		})

		switch choice {
		case 0:
			attackMenu(ctrl)
		case 1:
			thresholdMenu(docker, cfg)
		case 2:
			loadtestMenu(docker)
		case 3:
			viewReports(store)
		case 4:
			viewResilienceHistory()
		case 5:
			viewPlugins()
		case 6:
			fmt.Println(tui.Yellow + "serving metrics on " + cfg.MetricsAddr + " - Ctrl+C to stop" + tui.Reset)
			m.Serve(cfg.MetricsAddr)
		default:
			return
		}
	}
}

func attackMenu(ctrl *controller.Controller) {
	tui.Clear()
	names := plugins.List()
	choice := tui.Menu("Choose an attack", names)
	if choice < 0 {
		return
	}
	attackName := names[choice]
	target := tui.AskDefault("Target container", "aut-api")

	params := plugins.Params{}
	switch attackName {
	case "latency":
		params["delay_ms"] = tui.AskDefault("Delay (ms)", "300")
		params["jitter_ms"] = tui.AskDefault("Jitter (ms)", "20")
		params["duration_s"] = tui.AskDefault("Duration (s)", "15")
	case "cpu":
		params["workers"] = tui.AskDefault("CPU workers", "2")
		params["duration_s"] = tui.AskDefault("Duration (s)", "15")
	case "container-kill":
		params["restart"] = tui.AskDefault("Restart after stopping? (true/false)", "true")
	}

	fmt.Println()
	tui.Spin(fmt.Sprintf("running %s against %s", attackName, target), func() error {
		return ctrl.RunAttack(context.Background(), attackName, target, params)
	})
	tui.Pause()
}

func thresholdMenu(docker *dockerapi.Client, cfg config.Config) {
	tui.Clear()
	choice := tui.Menu("Choose what to threshold-test", []string{"latency", "cpu"})
	if choice < 0 {
		return
	}
	target := tui.AskDefault("Target container", "aut-api")

	var result threshold.Result
	var runErr error

	fmt.Println()
	tui.Spin("escalating attack intensity to find the breaking point", func() error {
		ctx := context.Background()
		if choice == 0 {
			net := network.NewManager(docker, "eth0")
			result, runErr = threshold.RunLatencyThreshold(ctx, net, target, cfg.TargetHealthURL)
		} else {
			result, runErr = threshold.RunCPUThreshold(ctx, docker, target, cfg.TargetHealthURL)
		}
		return runErr
	})

	if runErr == nil {
		fmt.Printf("\n%sbreaking point: %d %s%s\n", tui.Green, result.BreakingPoint, result.Unit, tui.Reset)
		hist := resilience.NewHistory("resilience-history.json")
		hist.Append("interactive", []threshold.Result{result})
	}
	tui.Pause()
}

func loadtestMenu(docker *dockerapi.Client) {
	tui.Clear()
	target := tui.AskDefault("Target container", "aut-api")
	url := tui.AskDefault("URL to load test", "http://localhost:8080/health")

	var result loadtest.Result
	var runErr error

	fmt.Println()
	tui.Spin("ramping up concurrent users", func() error {
		result, runErr = loadtest.Run(context.Background(), docker, target, url)
		return runErr
	})

	if runErr == nil {
		fmt.Printf("\n%smax concurrent users tolerated: %d%s\n", tui.Green, result.MaxConcurrentUsers, tui.Reset)
		fmt.Printf("requests/sec at that level: %.1f\n", result.RequestsPerSecond)
		fmt.Printf("avg latency: %d ms\n", result.AvgLatencyMs)
		fmt.Printf("error rate: %.1f%%\n", result.ErrorRatePercent)
		fmt.Printf("CPU usage: %.1f%%\n", result.CPUPercent)
		fmt.Printf("memory usage: %.1f MB\n", result.MemUsedMB)
	}
	tui.Pause()
}

func viewReports(store *reports.Store) {
	tui.Clear()
	list, err := store.List()
	if err != nil {
		fmt.Println(tui.Red + "error: " + err.Error() + tui.Reset)
	} else if len(list) == 0 {
		fmt.Println(tui.Gray + "no attacks recorded yet" + tui.Reset)
	} else {
		reports.Print(list)
	}
	tui.Pause()
}

func viewResilienceHistory() {
	tui.Clear()
	hist := resilience.NewHistory("resilience-history.json")
	regs, err := hist.CheckRegressions(20.0)
	if err != nil {
		fmt.Println(tui.Red + "error: " + err.Error() + tui.Reset)
	} else if len(regs) == 0 {
		fmt.Println(tui.Green + "no resilience regressions detected against the last run" + tui.Reset)
	} else {
		resilience.PrintRegressions(regs)
	}
	tui.Pause()
}

func viewPlugins() {
	tui.Clear()
	fmt.Println(tui.Bold + "Registered attack plugins" + tui.Reset)
	for _, name := range plugins.List() {
		p, _ := plugins.Get(name)
		fmt.Printf("  %s%-16s%s %s\n", tui.Cyan, name, tui.Reset, p.Describe())
	}
	_ = time.Now
	tui.Pause()
}
