package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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

var metricsRunning = false

func runInteractiveMenu(cfg config.Config, docker *dockerapi.Client, ctrl *controller.Controller, store *reports.Store, m *metrics.Collector) {
	for {
		tui.Clear()
		tui.Banner()
		printReminders(cfg, metricsRunning)

		metricsOpt := "Start metrics server in the background (for Grafana/Prometheus)"
		if metricsRunning {
			metricsOpt = "Stop metrics server (currently running in background)"
		}

		choice := tui.Menu("Main Menu", []string{
			"Check system status (Docker / test app / dashboard)",
			"Run a single attack (container-kill / latency / cpu)",
			"Find resilience breaking point (auto-escalating threshold test)",
			"Run a load test (find max concurrent users)",
			"Run the FULL test suite (all attacks + threshold + load test, one command)",
			"View attack report history",
			"View resilience history",
			"Show registered attack plugins",
			metricsOpt,
		})

		switch choice {
		case 0:
			checkSystemStatus(cfg)
		case 1:
			attackMenu(ctrl)
		case 2:
			thresholdMenu(docker, cfg)
		case 3:
			loadtestMenu(docker)
		case 4:
			runFullSuite(docker, ctrl, cfg)
		case 5:
			viewReports(store)
		case 6:
			viewResilienceHistory()
		case 7:
			viewPlugins()
		case 8:
			toggleMetricsBackground(cfg, m)
		default:
			if metricsRunning {
				fmt.Println()
				fmt.Println(tui.Yellow + "the metrics server is still running in the background." + tui.Reset)
				confirm := tui.AskDefault("quitting will stop it too - quit anyway? (y/n)", "n")
				if confirm != "y" && confirm != "yes" {
					continue
				}
			}
			return
		}
	}
}

func printReminders(cfg config.Config, metricsOn bool) {
	fmt.Println(tui.Gray + "  before running attacks: make sure Docker is running and the" + tui.Reset)
	fmt.Println(tui.Gray + "  test app is up (cd aut && docker compose up -d)" + tui.Reset)
	if metricsOn {
		fmt.Println(tui.Green + "  metrics server: running in background on " + cfg.MetricsAddr + tui.Reset)
	} else {
		fmt.Println(tui.Gray + "  metrics server: not running - start it from the menu to feed Grafana" + tui.Reset)
	}
	fmt.Println(tui.Gray + "  dashboard: http://localhost:3001  (Grafana, once monitoring stack is up)" + tui.Reset)
	fmt.Println()
}

func checkSystemStatus(cfg config.Config) {
	tui.Clear()
	fmt.Println(tui.Bold + "System status" + tui.Reset)
	fmt.Println()

	client := http.Client{Timeout: 3 * time.Second}

	fmt.Print("test app (" + cfg.TargetHealthURL + ")... ")
	resp, err := client.Get(cfg.TargetHealthURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		fmt.Println(tui.Red + "NOT REACHABLE" + tui.Reset)
		fmt.Println(tui.Yellow + "  fix: cd aut && docker compose up -d --build" + tui.Reset)
	} else {
		fmt.Println(tui.Green + "OK" + tui.Reset)
		resp.Body.Close()
	}

	fmt.Print("Grafana dashboard (http://localhost:3001)... ")
	resp2, err2 := client.Get("http://localhost:3001")
	if err2 != nil {
		fmt.Println(tui.Yellow + "NOT RUNNING" + tui.Reset)
		fmt.Println(tui.Yellow + "  fix: cd monitoring && docker compose up -d" + tui.Reset)
	} else {
		fmt.Println(tui.Green + "OK" + tui.Reset)
		resp2.Body.Close()
	}

	fmt.Print("Prometheus (http://localhost:9091)... ")
	resp3, err3 := client.Get("http://localhost:9091")
	if err3 != nil {
		fmt.Println(tui.Yellow + "NOT RUNNING" + tui.Reset)
		fmt.Println(tui.Yellow + "  fix: cd monitoring && docker compose up -d" + tui.Reset)
	} else {
		fmt.Println(tui.Green + "OK" + tui.Reset)
		resp3.Body.Close()
	}

	fmt.Print("metrics server (background, this session)... ")
	if metricsRunning {
		fmt.Println(tui.Green + "RUNNING" + tui.Reset)
	} else {
		fmt.Println(tui.Yellow + "NOT STARTED" + tui.Reset)
		fmt.Println(tui.Yellow + "  fix: start it from the main menu (option 9)" + tui.Reset)
	}

	tui.Pause()
}

func runFullSuite(docker *dockerapi.Client, ctrl *controller.Controller, cfg config.Config) {
	tui.Clear()
	fmt.Println(tui.Bold + "Running the full test suite - this will take a few minutes" + tui.Reset)
	fmt.Println()

	target := tui.AskDefault("Target container", "aut-api")
	fmt.Println()

	tui.Spin("container-kill attack", func() error {
		return ctrl.RunAttack(context.Background(), "container-kill", target, plugins.Params{})
	})

	tui.Spin("latency attack (300ms, 15s)", func() error {
		return ctrl.RunAttack(context.Background(), "latency", target, plugins.Params{
			"delay_ms": "300", "jitter_ms": "20", "duration_s": "15",
		})
	})

	tui.Spin("cpu attack (2 workers, 15s)", func() error {
		return ctrl.RunAttack(context.Background(), "cpu", target, plugins.Params{
			"workers": "2", "duration_s": "15",
		})
	})

	var latResult threshold.Result
	tui.Spin("resilience threshold test (latency)", func() error {
		net := network.NewManager(docker, "eth0")
		var err error
		latResult, err = threshold.RunLatencyThreshold(context.Background(), net, target, cfg.TargetHealthURL)
		return err
	})
	if latResult.BreakingPoint > 0 {
		hist := resilience.NewHistory("resilience-history.json")
		hist.Append("interactive-full-suite", []threshold.Result{latResult})
	}

	var loadResult loadtest.Result
	tui.Spin("load test (concurrent users)", func() error {
		var err error
		loadResult, err = loadtest.Run(context.Background(), docker, target, cfg.TargetHealthURL)
		return err
	})

	fmt.Println()
	fmt.Println(tui.Bold + tui.Green + "Full suite complete." + tui.Reset)
	fmt.Println()
	fmt.Println(tui.Bold + "Summary:" + tui.Reset)
	fmt.Printf("  latency breaking point:     %d ms\n", latResult.BreakingPoint)
	fmt.Printf("  max concurrent users:       %d\n", loadResult.MaxConcurrentUsers)
	fmt.Printf("  CPU usage at max load:      %.1f%%\n", loadResult.CPUPercent)
	fmt.Printf("  memory usage at max load:   %.1f MB\n", loadResult.MemUsedMB)
	fmt.Println()
	fmt.Println(tui.Cyan + "View full results and trends in Grafana: http://localhost:3001" + tui.Reset)
	fmt.Println(tui.Cyan + "(make sure the monitoring stack is up: cd monitoring && docker compose up -d)" + tui.Reset)

	tui.Pause()
}

// attackMenu now offers every plugin individually, plus "all three
// sequentially", plus a continuous mode that keeps running until the user
// presses Ctrl+C.
func attackMenu(ctrl *controller.Controller) {
	tui.Clear()
	names := plugins.List()
	options := append([]string{}, names...)
	options = append(options, "ALL THREE (container-kill, latency, cpu - one after another)")

	choice := tui.Menu("Choose an attack", options)
	if choice < 0 {
		return
	}

	target := tui.AskDefault("Target container", "aut-api")
	runAll := choice == len(options)-1

	var attackName string
	params := plugins.Params{}
	if !runAll {
		attackName = names[choice]
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
	}

	modeChoice := tui.Menu("How should it run?", []string{
		"Once",
		"Continuously, until I stop it (Ctrl+C)",
	})
	if modeChoice < 0 {
		return
	}
	continuous := modeChoice == 1

	fmt.Println()

	runOnce := func() error {
		if runAll {
			if err := ctrl.RunAttack(context.Background(), "container-kill", target, plugins.Params{}); err != nil {
				return err
			}
			if err := ctrl.RunAttack(context.Background(), "latency", target, plugins.Params{
				"delay_ms": "300", "jitter_ms": "20", "duration_s": "10",
			}); err != nil {
				return err
			}
			return ctrl.RunAttack(context.Background(), "cpu", target, plugins.Params{
				"workers": "2", "duration_s": "10",
			})
		}
		return ctrl.RunAttack(context.Background(), attackName, target, params)
	}

	if !continuous {
		label := attackName
		if runAll {
			label = "all three attacks"
		}
		tui.Spin(fmt.Sprintf("running %s against %s", label, target), runOnce)
		tui.Pause()
		return
	}

	fmt.Println(tui.Yellow + "running continuously - press Ctrl+C to stop" + tui.Reset)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	round := 1
	for {
		select {
		case <-sigCh:
			fmt.Println()
			fmt.Printf("%sstopped after %d round(s)%s\n", tui.Green, round-1, tui.Reset)
			tui.Pause()
			return
		default:
		}

		label := attackName
		if runAll {
			label = "all three attacks"
		}
		tui.Spin(fmt.Sprintf("round %d: %s against %s", round, label, target), runOnce)
		round++
		time.Sleep(2 * time.Second)
	}
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
	tui.Pause()
}

var metricsServer *http.Server

func toggleMetricsBackground(cfg config.Config, m *metrics.Collector) {
	tui.Clear()
	if metricsRunning {
		if metricsServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = metricsServer.Shutdown(ctx)
			metricsServer = nil
		}
		metricsRunning = false
		fmt.Println(tui.Yellow + "metrics server stopped." + tui.Reset)
		tui.Pause()
		return
	}

	ln, err := m.TryListen(cfg.MetricsAddr)
	if err != nil {
		fmt.Println(tui.Red + "failed to start metrics server on " + cfg.MetricsAddr + ": " + err.Error() + tui.Reset)
		fmt.Println(tui.Yellow + "hint: another process (e.g. background gremlin) is using port " + cfg.MetricsAddr + tui.Reset)
		tui.Pause()
		return
	}

	metricsServer = &http.Server{
		Handler: m.Handler(),
	}
	metricsRunning = true
	go func() {
		if err := metricsServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			metricsRunning = false
			metricsServer = nil
		}
	}()

	fmt.Println(tui.Green + "metrics server started in the background on " + cfg.MetricsAddr + tui.Reset)
	fmt.Println(tui.Gray + "it stays running while you use other options until turned off via option 9." + tui.Reset)
	fmt.Println()
	fmt.Println(tui.Cyan + "Prometheus should scrape it at http://172.17.0.1" + cfg.MetricsAddr + "/metrics" + tui.Reset)
	fmt.Println(tui.Cyan + "View the dashboard at http://localhost:3001" + tui.Reset)
	tui.Pause()
}
