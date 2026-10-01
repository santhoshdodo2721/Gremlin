package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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

		choice := tui.Menu("Main Menu", []string{
			"🚀 Step-by-Step Guided Attack Wizard (Recommended for beginners)",
			"⚡ Quick Attack (Select attack plugin directly)",
			"📊 Check system status (Docker / containers / dashboard)",
			"📈 Find resilience breaking point (Auto-escalating threshold test)",
			"👥 Run a load test (Find max concurrent users)",
			"🧪 Run the FULL test suite (Attacks + threshold + load test)",
			"📜 View attack report history",
			"📉 View resilience history",
			"🔌 Show registered attack plugins",
			"📡 Start metrics server in the background (for Grafana/Prometheus)",
		})

		switch choice {
		case 0:
			// Auto-start metrics server so Grafana immediately receives telemetry
			if !metricsRunning {
				metricsRunning = true
				go m.Serve(cfg.MetricsAddr)
			}
			runGuidedWizard(docker, ctrl, cfg)
		case 1:
			attackMenu(docker, ctrl, cfg)
		case 2:
			checkSystemStatus(cfg, docker)
		case 3:
			thresholdMenu(docker, cfg)
		case 4:
			loadtestMenu(docker, cfg)
		case 5:
			runFullSuite(docker, ctrl, cfg)
		case 6:
			viewReports(store)
		case 7:
			viewResilienceHistory()
		case 8:
			viewPlugins()
		case 9:
			startMetricsBackground(cfg, m)
		default:
			return
		}
	}
}

func printReminders(cfg config.Config, metricsOn bool) {
	fmt.Println(tui.Gray + "  universal chaos engineering: attacks any Docker container" + tui.Reset)
	if metricsOn {
		fmt.Println(tui.Green + "  metrics server: running in background on " + cfg.MetricsAddr + tui.Reset)
	} else {
		fmt.Println(tui.Yellow + "  metrics server: not started yet (option 10 starts it, or start guided wizard)" + tui.Reset)
	}
	fmt.Println(tui.Gray + "  dashboard: http://localhost:3001  (Grafana, once monitoring stack is up)" + tui.Reset)
	fmt.Println()
}

// runGuidedWizard walks a user through a chaos attack step-by-step
// with plain-language explanations, safe presets, and pre-flight checks.
func runGuidedWizard(docker *dockerapi.Client, ctrl *controller.Controller, cfg config.Config) {
	tui.Clear()
	fmt.Println(tui.Bold + tui.Red + "  🚀 STEP-BY-STEP GUIDED CHAOS WIZARD" + tui.Reset)
	fmt.Println(tui.Gray + "  Test the resilience of any Docker container without prior chaos experience" + tui.Reset)

	// STEP 1: Discover & Select Target Container
	tui.StepHeader(1, 4, "Choose Target Container")
	tui.Info("Scanning your local Docker daemon for running containers...")
	containers, err := docker.ListContainers(context.Background())
	if err != nil {
		fmt.Println(tui.Red + "Error listing containers: " + err.Error() + tui.Reset)
		tui.Pause()
		return
	}
	if len(containers) == 0 {
		tui.Warning("No running Docker containers detected. Start your container or application first!")
		tui.Pause()
		return
	}

	var containerNames []string
	var menuItems []string
	for _, c := range containers {
		name := c.ID[:12]
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		containerNames = append(containerNames, name)
		menuItems = append(menuItems, fmt.Sprintf("%-28s [%s]", name, c.Image))
	}
	menuItems = append(menuItems, "Enter custom container name or ID manually...")

	cChoice := tui.Menu("Which container would you like to test?", menuItems)
	if cChoice < 0 {
		return
	}

	var target string
	if cChoice < len(containerNames) {
		target = containerNames[cChoice]
	} else {
		target = tui.AskDefault("Container name or ID", "aut-api")
	}
	tui.Success("Target container selected: " + target)

	// STEP 2: Choose Failure Scenario (in Plain English)
	tui.Clear()
	tui.StepHeader(2, 4, "Choose Failure Scenario")
	tui.Info("Select what real-world failure condition you want to simulate against " + target + ":")

	scenarios := []string{
		"🐢 Network Lag / Congestion   (Simulate slow cross-region/cloud latency)",
		"📉 Network Packet Loss         (Simulate unstable connection / dropped packets)",
		"⚡ CPU Starvation             (Simulate runaway loop / noisy neighbor CPU spike)",
		"⏸️  Service Freeze / Hang       (Simulate thread deadlock / GC freeze)",
		"💥 Hard Crash & Restart        (Simulate container crash / OOM killer)",
		"🔀 Packet Corruption          (Simulate faulty network interface / bad data)",
	}

	sChoice := tui.Menu("Choose a failure scenario to inject", scenarios)
	if sChoice < 0 {
		return
	}

	var attackName string
	var attackTitle string
	switch sChoice {
	case 0:
		attackName = "latency"
		attackTitle = "Network Latency (Lag)"
	case 1:
		attackName = "packet-loss"
		attackTitle = "Network Packet Loss"
	case 2:
		attackName = "cpu"
		attackTitle = "CPU Starvation (Throttle)"
	case 3:
		attackName = "container-pause"
		attackTitle = "Service Freeze (Pause)"
	case 4:
		attackName = "container-kill"
		attackTitle = "Hard Crash & Restart"
	case 5:
		attackName = "corruption"
		attackTitle = "Network Packet Corruption"
	}
	tui.Success("Scenario selected: " + attackTitle)

	// STEP 3: Choose Attack Intensity Presets
	tui.Clear()
	tui.StepHeader(3, 4, "Choose Attack Intensity")
	tui.Info("Beginner presets provide safe, predictable chaos tests:")

	intensities := []string{
		"🟢 Mild     (Gentle test: quick duration, moderate stress)",
		"🟡 Moderate (Realistic test: typical production incident level)",
		"🔴 Heavy    (High stress: pushes service near its capacity limit)",
		"⚙️  Custom   (Manually specify exact delay, percentage, or duration)",
	}

	iChoice := tui.Menu("Choose intensity level", intensities)
	if iChoice < 0 {
		return
	}

	params := plugins.Params{}
	var settingsSummary string

	switch attackName {
	case "latency":
		switch iChoice {
		case 0: // Mild
			params["delay_ms"] = "150"
			params["jitter_ms"] = "15"
			params["duration_s"] = "8"
			settingsSummary = "150ms delay (+/-15ms jitter) for 8 seconds"
		case 1: // Moderate
			params["delay_ms"] = "350"
			params["jitter_ms"] = "30"
			params["duration_s"] = "15"
			settingsSummary = "350ms delay (+/-30ms jitter) for 15 seconds"
		case 2: // Heavy
			params["delay_ms"] = "1000"
			params["jitter_ms"] = "100"
			params["duration_s"] = "20"
			settingsSummary = "1000ms (1s) delay (+/-100ms jitter) for 20 seconds"
		case 3: // Custom
			params["delay_ms"] = tui.AskDefault("Delay in milliseconds", "300")
			params["jitter_ms"] = tui.AskDefault("Jitter in milliseconds", "20")
			params["duration_s"] = tui.AskDefault("Duration in seconds", "15")
			settingsSummary = fmt.Sprintf("%sms delay for %ss", params["delay_ms"], params["duration_s"])
		}

	case "packet-loss":
		switch iChoice {
		case 0: // Mild
			params["percent"] = "10"
			params["duration_s"] = "8"
			settingsSummary = "10% packet loss for 8 seconds"
		case 1: // Moderate
			params["percent"] = "25"
			params["duration_s"] = "15"
			settingsSummary = "25% packet loss for 15 seconds"
		case 2: // Heavy
			params["percent"] = "50"
			params["duration_s"] = "20"
			settingsSummary = "50% packet loss for 20 seconds"
		case 3: // Custom
			params["percent"] = tui.AskDefault("Packet loss percentage (e.g. 20)", "20")
			params["duration_s"] = tui.AskDefault("Duration in seconds", "15")
			settingsSummary = fmt.Sprintf("%s%% loss for %ss", params["percent"], params["duration_s"])
		}

	case "cpu":
		params["method"] = "throttle" // Guaranteed to work on ANY container without stress-ng
		switch iChoice {
		case 0: // Mild
			params["quota_pct"] = "30"
			params["duration_s"] = "8"
			settingsSummary = "Throttle to 30% CPU quota for 8 seconds"
		case 1: // Moderate
			params["quota_pct"] = "10"
			params["duration_s"] = "15"
			settingsSummary = "Throttle to 10% CPU quota (severe) for 15 seconds"
		case 2: // Heavy
			params["quota_pct"] = "5"
			params["duration_s"] = "20"
			settingsSummary = "Throttle to 5% CPU quota (extreme starvation) for 20 seconds"
		case 3: // Custom
			params["quota_pct"] = tui.AskDefault("CPU quota % (1-100)", "10")
			params["duration_s"] = tui.AskDefault("Duration in seconds", "15")
			settingsSummary = fmt.Sprintf("Throttle to %s%% CPU quota for %ss", params["quota_pct"], params["duration_s"])
		}

	case "container-pause":
		switch iChoice {
		case 0: // Mild
			params["duration_s"] = "5"
			settingsSummary = "Freeze container for 5 seconds"
		case 1: // Moderate
			params["duration_s"] = "10"
			settingsSummary = "Freeze container for 10 seconds"
		case 2: // Heavy
			params["duration_s"] = "20"
			settingsSummary = "Freeze container for 20 seconds"
		case 3: // Custom
			params["duration_s"] = tui.AskDefault("Freeze duration in seconds", "10")
			settingsSummary = fmt.Sprintf("Freeze container for %ss", params["duration_s"])
		}

	case "container-kill":
		params["restart"] = "true"
		settingsSummary = "Crash container immediately, then restart cleanly"

	case "corruption":
		switch iChoice {
		case 0:
			params["percent"] = "5"
			params["duration_s"] = "8"
			settingsSummary = "5% packet corruption for 8 seconds"
		case 1:
			params["percent"] = "15"
			params["duration_s"] = "15"
			settingsSummary = "15% packet corruption for 15 seconds"
		case 2:
			params["percent"] = "35"
			params["duration_s"] = "20"
			settingsSummary = "35% packet corruption for 20 seconds"
		case 3:
			params["percent"] = tui.AskDefault("Packet corruption percentage (e.g. 10)", "10")
			params["duration_s"] = tui.AskDefault("Duration in seconds", "15")
			settingsSummary = fmt.Sprintf("%s%% corruption for %ss", params["percent"], params["duration_s"])
		}
	}

	// STEP 4: Recovery Verification (Health Check)
	tui.Clear()
	tui.StepHeader(4, 4, "Recovery Verification")
	tui.Info("How should Gremlin verify that your service has recovered after the attack?")

	suggestedHealth := "http://localhost:80/"
	if strings.Contains(target, "aut-api") {
		suggestedHealth = "http://localhost:8080/health"
	}

	hOptions := []string{
		fmt.Sprintf("Use auto-detected endpoint (%s)", suggestedHealth),
		"Specify a custom URL",
		"Skip recovery check (run attack only)",
	}

	hChoice := tui.Menu("Choose verification method", hOptions)
	var healthURL string
	switch hChoice {
	case 0:
		healthURL = suggestedHealth
	case 1:
		healthURL = tui.AskDefault("Enter URL to check", "http://localhost:80/")
	case 2:
		healthURL = ""
	default:
		healthURL = suggestedHealth
	}

	// PRE-FLIGHT VERIFICATION
	tui.Clear()
	fmt.Println(tui.Bold + "================================================================" + tui.Reset)
	fmt.Println(tui.Bold + tui.Cyan + "                   PRE-FLIGHT TEST SUMMARY" + tui.Reset)
	fmt.Println(tui.Bold + "================================================================" + tui.Reset)
	fmt.Printf("  • Target Container : %s%s%s\n", tui.Bold, target, tui.Reset)
	fmt.Printf("  • Failure Scenario : %s%s%s\n", tui.Bold, attackTitle, tui.Reset)
	fmt.Printf("  • Attack Settings  : %s\n", settingsSummary)
	if healthURL != "" {
		fmt.Printf("  • Recovery URL     : %s\n", healthURL)
		client := http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(healthURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			tui.Success("Pre-flight check passed: Endpoint is responsive!")
		} else {
			tui.Warning("Endpoint did not return HTTP 200 right now (test will still proceed).")
		}
	} else {
		fmt.Println("  • Recovery URL     : [Skipped]")
	}
	fmt.Println(tui.Bold + "================================================================" + tui.Reset)
	fmt.Println()

	if !tui.AskConfirm("Ready to launch this chaos test?", true) {
		fmt.Println("Attack cancelled.")
		tui.Pause()
		return
	}

	// EXECUTION
	fmt.Println()
	tui.Info("Starting fault injection now. Watch how your application responds...")
	start := time.Now()
	var attackErr error

	tui.Spin(fmt.Sprintf("Injecting %s into %s", attackTitle, target), func() error {
		attackErr = ctrl.RunAttackWithHealthURL(context.Background(), attackName, target, params, healthURL)
		return attackErr
	})

	elapsed := time.Since(start).Round(time.Millisecond)

	// REPORT / DIAGNOSIS
	tui.Clear()
	if attackErr != nil {
		fmt.Println(tui.Bold + tui.Red + "================================================================" + tui.Reset)
		fmt.Println(tui.Bold + tui.Red + "                  CHAOS TEST RESULT: FAILED" + tui.Reset)
		fmt.Println(tui.Bold + tui.Red + "================================================================" + tui.Reset)
		fmt.Printf("  Target      : %s\n", target)
		fmt.Printf("  Attack      : %s\n", attackTitle)
		fmt.Printf("  Error Reason: %v\n", attackErr)
		fmt.Println()
		tui.Info("Tip: Make sure the target container is running.")
	} else {
		fmt.Println(tui.Bold + tui.Green + "================================================================" + tui.Reset)
		fmt.Println(tui.Bold + tui.Green + "                  CHAOS TEST RESULT: PASSED" + tui.Reset)
		fmt.Println(tui.Bold + tui.Green + "================================================================" + tui.Reset)
		fmt.Printf("  Target Container : %s%s%s\n", tui.Bold, target, tui.Reset)
		fmt.Printf("  Failure Injected : %s\n", attackTitle)
		fmt.Printf("  Fault Duration   : %s\n", elapsed)
		if healthURL != "" {
			fmt.Printf("  Recovery Status  : Service successfully recovered!\n")
		}
		fmt.Println()
		fmt.Println(tui.Bold + "  What this means for your application:" + tui.Reset)
		fmt.Println("  ✓ Gremlin successfully stressed this container and fully reverted the state.")
		fmt.Println("  ✓ The target container is running normally.")
		fmt.Println()
		fmt.Println(tui.Cyan + "  📈 View real-time graphs in Grafana: http://localhost:3001" + tui.Reset)
		fmt.Println(tui.Gray + "  (Attack telemetry has been recorded to Prometheus & Grafana)" + tui.Reset)
	}
	fmt.Println(tui.Bold + "================================================================" + tui.Reset)
	tui.Pause()
}

func selectTargetAndHealth(docker *dockerapi.Client, defaultHealth string) (string, string) {
	containers, err := docker.ListContainers(context.Background())
	var options []string
	var targets []string
	if err == nil && len(containers) > 0 {
		for _, c := range containers {
			name := c.ID[:12]
			if len(c.Names) > 0 {
				name = strings.TrimPrefix(c.Names[0], "/")
			}
			targets = append(targets, name)
			options = append(options, fmt.Sprintf("%-28s [%s]", name, c.Image))
		}
		options = append(options, "Type container name manually...")
		choice := tui.Menu("Select Target Container", options)
		var target string
		if choice >= 0 && choice < len(targets) {
			target = targets[choice]
		} else if choice == len(targets) {
			target = tui.AskDefault("Target container name or ID", "aut-api")
		} else {
			target = "aut-api"
		}

		suggestedHealth := defaultHealth
		if strings.Contains(target, "front-end") || strings.Contains(target, "edge-router") {
			suggestedHealth = "http://localhost:80/"
		} else if strings.Contains(target, "aut-api") {
			suggestedHealth = "http://localhost:8080/health"
		}
		healthURL := tui.AskDefault("Health URL to verify recovery (leave empty to skip)", suggestedHealth)
		return target, healthURL
	}

	target := tui.AskDefault("Target container", "aut-api")
	healthURL := tui.AskDefault("Health URL to verify recovery (leave empty to skip)", defaultHealth)
	return target, healthURL
}

func checkSystemStatus(cfg config.Config, docker *dockerapi.Client) {
	tui.Clear()
	fmt.Println(tui.Bold + "System status" + tui.Reset)
	fmt.Println()

	client := http.Client{Timeout: 3 * time.Second}

	fmt.Print("Docker daemon (/var/run/docker.sock)... ")
	containers, err := docker.ListContainers(context.Background())
	if err != nil {
		fmt.Println(tui.Red + "UNREACHABLE" + tui.Reset)
		fmt.Printf("  error: %v\n", err)
	} else {
		fmt.Printf("%sOK%s (%d running containers detected)\n", tui.Green, tui.Reset, len(containers))
		for _, c := range containers {
			name := c.ID[:12]
			if len(c.Names) > 0 {
				name = strings.TrimPrefix(c.Names[0], "/")
			}
			fmt.Printf("    • %-28s [%s]\n", name, c.Status)
		}
		fmt.Println()
	}

	if cfg.TargetHealthURL != "" {
		fmt.Print("configured health URL (" + cfg.TargetHealthURL + ")... ")
		resp, err := client.Get(cfg.TargetHealthURL)
		if err != nil || resp.StatusCode != http.StatusOK {
			fmt.Println(tui.Yellow + "NOT REACHABLE" + tui.Reset)
			fmt.Println(tui.Gray + "  (normal if testing an external app - specify your app's health URL during attack)" + tui.Reset)
		} else {
			fmt.Println(tui.Green + "OK" + tui.Reset)
			resp.Body.Close()
		}
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

	if !metricsRunning {
		fmt.Println()
		fmt.Println(tui.Yellow + "note: metrics server is not running from this menu yet - Prometheus" + tui.Reset)
		fmt.Println(tui.Yellow + "has nothing to scrape until you start it (option 10)." + tui.Reset)
	}

	tui.Pause()
}

func runFullSuite(docker *dockerapi.Client, ctrl *controller.Controller, cfg config.Config) {
	tui.Clear()
	fmt.Println(tui.Bold + "Running the full test suite" + tui.Reset)
	fmt.Println()

	target, healthURL := selectTargetAndHealth(docker, cfg.TargetHealthURL)
	fmt.Println()

	tui.Spin("container-pause attack (10s)", func() error {
		return ctrl.RunAttackWithHealthURL(context.Background(), "container-pause", target, plugins.Params{"duration_s": "10"}, healthURL)
	})

	tui.Spin("latency attack (300ms, 15s)", func() error {
		return ctrl.RunAttackWithHealthURL(context.Background(), "latency", target, plugins.Params{
			"delay_ms": "300", "jitter_ms": "20", "duration_s": "15",
		}, healthURL)
	})

	tui.Spin("cpu attack (cgroup throttle, 15s)", func() error {
		return ctrl.RunAttackWithHealthURL(context.Background(), "cpu", target, plugins.Params{
			"quota_pct": "10", "duration_s": "15",
		}, healthURL)
	})

	var latResult threshold.Result
	tui.Spin("resilience threshold test (latency)", func() error {
		net := network.NewManager(docker, "eth0")
		var err error
		latResult, err = threshold.RunLatencyThreshold(context.Background(), net, target, healthURL)
		return err
	})
	if latResult.BreakingPoint > 0 {
		hist := resilience.NewHistory("resilience-history.json")
		hist.Append("interactive-full-suite", []threshold.Result{latResult})
	}

	var loadResult loadtest.Result
	if healthURL != "" {
		tui.Spin("load test (concurrent users)", func() error {
			var err error
			loadResult, err = loadtest.Run(context.Background(), docker, target, healthURL)
			return err
		})
	}

	fmt.Println()
	fmt.Println(tui.Bold + tui.Green + "Full suite complete." + tui.Reset)
	fmt.Println()
	fmt.Println(tui.Bold + "Summary:" + tui.Reset)
	fmt.Printf("  latency breaking point:     %d ms\n", latResult.BreakingPoint)
	if healthURL != "" {
		fmt.Printf("  max concurrent users:       %d\n", loadResult.MaxConcurrentUsers)
		fmt.Printf("  CPU usage at max load:      %.1f%%\n", loadResult.CPUPercent)
		fmt.Printf("  memory usage at max load:   %.1f MB\n", loadResult.MemUsedMB)
	}
	fmt.Println()
	fmt.Println(tui.Cyan + "View full results and trends in Grafana: http://localhost:3001" + tui.Reset)

	tui.Pause()
}

func attackMenu(docker *dockerapi.Client, ctrl *controller.Controller, cfg config.Config) {
	tui.Clear()
	names := plugins.List()
	choice := tui.Menu("Choose an attack", names)
	if choice < 0 {
		return
	}
	attackName := names[choice]
	target, healthURL := selectTargetAndHealth(docker, cfg.TargetHealthURL)

	params := plugins.Params{}
	switch attackName {
	case "latency":
		params["delay_ms"] = tui.AskDefault("Delay (ms)", "300")
		params["jitter_ms"] = tui.AskDefault("Jitter (ms)", "20")
		params["duration_s"] = tui.AskDefault("Duration (s)", "15")
	case "packet-loss":
		params["percent"] = tui.AskDefault("Packet loss percentage", "20")
		params["duration_s"] = tui.AskDefault("Duration (s)", "15")
	case "corruption":
		params["percent"] = tui.AskDefault("Packet corruption percentage", "10")
		params["duration_s"] = tui.AskDefault("Duration (s)", "15")
	case "cpu":
		params["method"] = tui.AskDefault("Method (auto / throttle / stress)", "auto")
		if params["method"] == "throttle" || params["method"] == "auto" {
			params["quota_pct"] = tui.AskDefault("CPU quota % for throttling (e.g. 10 = 10% of 1 core)", "10")
		}
		params["workers"] = tui.AskDefault("CPU workers (for stress mode)", "2")
		params["duration_s"] = tui.AskDefault("Duration (s)", "15")
	case "container-kill":
		params["restart"] = tui.AskDefault("Restart after stopping? (true/false)", "true")
	case "container-pause":
		params["duration_s"] = tui.AskDefault("Freeze duration (s)", "15")
	}

	fmt.Println()
	tui.Spin(fmt.Sprintf("running %s against %s", attackName, target), func() error {
		return ctrl.RunAttackWithHealthURL(context.Background(), attackName, target, params, healthURL)
	})
	tui.Pause()
}

func thresholdMenu(docker *dockerapi.Client, cfg config.Config) {
	tui.Clear()
	choice := tui.Menu("Choose what to threshold-test", []string{"latency", "cpu"})
	if choice < 0 {
		return
	}
	target, healthURL := selectTargetAndHealth(docker, cfg.TargetHealthURL)

	var result threshold.Result
	var runErr error

	fmt.Println()
	tui.Spin("escalating attack intensity to find the breaking point", func() error {
		ctx := context.Background()
		if choice == 0 {
			net := network.NewManager(docker, "eth0")
			result, runErr = threshold.RunLatencyThreshold(ctx, net, target, healthURL)
		} else {
			result, runErr = threshold.RunCPUThreshold(ctx, docker, target, healthURL)
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

func loadtestMenu(docker *dockerapi.Client, cfg config.Config) {
	tui.Clear()
	target, defaultHealth := selectTargetAndHealth(docker, cfg.TargetHealthURL)
	url := tui.AskDefault("URL to load test", defaultHealth)

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

func startMetricsBackground(cfg config.Config, m *metrics.Collector) {
	tui.Clear()
	if metricsRunning {
		fmt.Println(tui.Yellow + "metrics server is already running on " + cfg.MetricsAddr + tui.Reset)
		tui.Pause()
		return
	}

	metricsRunning = true
	go func() {
		m.Serve(cfg.MetricsAddr)
	}()

	fmt.Println(tui.Green + "metrics server started in the background on " + cfg.MetricsAddr + tui.Reset)
	fmt.Println(tui.Gray + "it will keep running for the rest of this session - you can use every" + tui.Reset)
	fmt.Println(tui.Gray + "other menu option normally while it serves Prometheus in the background." + tui.Reset)
	fmt.Println()
	fmt.Println(tui.Cyan + "Prometheus scrape endpoint: http://localhost" + cfg.MetricsAddr + "/metrics" + tui.Reset)
	fmt.Println(tui.Cyan + "View the dashboard at http://localhost:3001" + tui.Reset)
	tui.Pause()
}
