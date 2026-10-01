package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
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
		printApplicationWorkspace(cfg, docker)
		choice := tui.MainMenu([]string{
			"Guided experiment",
			"Continuous testing",
			"Results & history",
			"Application status",
			"Advanced tools",
			"View running services",
		})
		switch choice {
		case 0:
			runGuidedWizard(docker, ctrl, cfg)
		case 1:
			continuousMenu(docker, ctrl, cfg)
		case 2:
			resultsMenu(store)
		case 3:
			checkSystemStatus(cfg, docker)
		case 4:
			advancedMenu(cfg, docker, ctrl, m)
		case 5:
			viewRunningServices(cfg, docker)
		default:
			return
		}
	}
}

func viewRunningServices(cfg config.Config, docker *dockerapi.Client) {
	tui.Clear()
	tui.Banner()
	fmt.Println(tui.Bold + tui.Red + "  RUNNING APPLICATION SERVICES" + tui.Reset)
	fmt.Println(tui.Gray + "  " + cfg.ApplicationDirectory + tui.Reset)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	containers, err := docker.ListApplicationContainers(ctx, cfg.ApplicationDirectory)
	if err != nil {
		tui.Warning("Could not load running services: " + err.Error())
	} else if len(containers) == 0 {
		tui.Info("No application services are running.")
	} else {
		fmt.Printf("\n  %d running services\n\n", len(containers))
		table := tabwriter.NewWriter(os.Stdout, 0, 4, 3, ' ', 0)
		fmt.Fprintln(table, "  SERVICE\tCONTAINER\tSTATUS")
		for _, container := range containers {
			service := container.Labels["com.docker.compose.service"]
			if service == "" {
				service = "—"
			}
			fmt.Fprintf(table, "  %s\t%s\t%s\n", service, container.DisplayName(), container.Status)
		}
		table.Flush()
	}
	fmt.Println()
	tui.Pause()
}

func resultsMenu(store *reports.Store) {
	for {
		tui.Clear()
		tui.Banner()
		choice := tui.Menu("View results", []string{
			"Test history",
			"Resilience history",
		})
		switch choice {
		case 0:
			viewReports(store)
		case 1:
			viewResilienceHistory()
		default:
			return
		}
	}
}

func advancedMenu(cfg config.Config, docker *dockerapi.Client, ctrl *controller.Controller, m *metrics.Collector) {
	for {
		tui.Clear()
		tui.Banner()
		choice := tui.Menu("Advanced tools", []string{
			"Custom test",
			"Service limits",
			"Load test",
			"Full test suite",
			"Attack types",
			"Local metrics server",
		})
		switch choice {
		case 0:
			attackMenu(docker, ctrl, cfg)
		case 1:
			thresholdMenu(docker, cfg)
		case 2:
			loadtestMenu(docker, cfg)
		case 3:
			runFullSuite(docker, ctrl, cfg)
		case 4:
			viewPlugins()
		case 5:
			startMetricsBackground(cfg, m)
		default:
			return
		}
	}
}

// runGuidedWizard walks a user through a chaos attack step-by-step
// with plain-language explanations, safe presets, and pre-flight checks.
func runGuidedWizard(docker *dockerapi.Client, ctrl *controller.Controller, cfg config.Config) {
	tui.Clear()
	tui.Banner()
	fmt.Println(tui.Bold + tui.Red + "  GUIDED EXPERIMENT" + tui.Reset)
	fmt.Println(tui.Gray + "  Choose an application service, a failure, and a recovery check." + tui.Reset)

	// STEP 1: Discover & Select Target Container
	tui.StepHeader(1, 4, "Choose Target Container")
	tui.Info("Discovering running services in this application...")
	containers, err := docker.ListApplicationContainers(context.Background(), cfg.ApplicationDirectory)
	if err != nil {
		fmt.Println(tui.Red + "Error listing containers: " + err.Error() + tui.Reset)
		tui.Pause()
		return
	}
	if len(containers) == 0 {
		tui.Warning("No application containers are running. Start them with: docker compose -f aut/microservices-demo/deploy/docker-compose/docker-compose.yml up -d")
		tui.Pause()
		return
	}

	var containerNames []string
	var menuItems []string
	for _, c := range containers {
		name := c.ID
		if len(name) > 12 {
			name = name[:12]
		}
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		containerNames = append(containerNames, name)
		menuItems = append(menuItems, fmt.Sprintf("%-28s [%s]", name, c.Image))
	}

	cChoice := tui.Menu("Which container would you like to test?", menuItems)
	if cChoice < 0 {
		return
	}

	target := containerNames[cChoice]
	tui.Success("Target container selected: " + target)

	// STEP 2: Choose Failure Scenario (in Plain English)
	tui.Clear()
	tui.Banner()
	tui.StepHeader(2, 4, "Choose Failure Scenario")
	tui.Info("Select what real-world failure condition you want to simulate against " + target + ":")

	scenarios := []string{
		"Network latency       Slow the service connection",
		"Packet loss           Drop a percentage of traffic",
		"CPU throttling        Limit available processing capacity",
		"Service freeze        Pause the application container",
		"Crash and restart     Stop and restart the service",
		"Packet corruption     Damage a percentage of traffic",
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
	tui.Banner()
	tui.StepHeader(3, 4, "Choose Attack Intensity")
	tui.Info("Beginner presets provide safe, predictable chaos tests:")

	intensities := []string{
		"Mild         Short duration, lower intensity",
		"Moderate     Longer duration, medium intensity",
		"Heavy        Longer duration, higher intensity",
		"Custom       Configure intensity and duration",
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
	tui.Banner()
	tui.StepHeader(4, 4, "Recovery Verification")
	tui.Info("How should Gremlin verify that your service has recovered after the attack?")

	suggestedHealth := cfg.TargetHealthURL

	hOptions := []string{
		fmt.Sprintf("Use suggested endpoint (%s)", suggestedHealth),
		"Specify a custom URL",
		"Skip recovery check (run attack only)",
	}

	hChoice := tui.Menu("Choose verification method", hOptions)
	var healthURL string
	switch hChoice {
	case 0:
		healthURL = suggestedHealth
	case 1:
		healthURL = tui.AskDefault("Enter URL to check", cfg.TargetHealthURL)
	case 2:
		healthURL = ""
	default:
		return
	}

	// PRE-FLIGHT VERIFICATION
	tui.Clear()
	tui.Banner()
	fmt.Println(tui.Bold + "================================================================" + tui.Reset)
	fmt.Println(tui.Bold + tui.Red + "                   PRE-FLIGHT TEST SUMMARY" + tui.Reset)
	fmt.Println(tui.Bold + "================================================================" + tui.Reset)
	fmt.Printf("  • Target Container : %s%s%s\n", tui.Bold, target, tui.Reset)
	fmt.Printf("  • Failure Scenario : %s%s%s\n", tui.Bold, attackTitle, tui.Reset)
	fmt.Printf("  • Attack Settings  : %s\n", settingsSummary)
	if healthURL != "" {
		fmt.Printf("  • Recovery URL     : %s\n", healthURL)
		client := http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(healthURL)
		if resp != nil {
			defer resp.Body.Close()
		}
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
	tui.Banner()
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
		if healthURL != "" {
			fmt.Println("  ✓ The health endpoint returned HTTP 200 after the test.")
		}
		fmt.Println()
		fmt.Println(tui.Red + "  📈 View real-time graphs in Grafana: http://localhost:3001" + tui.Reset)
		fmt.Println(tui.Gray + "  Reports saved locally; monitoring requires a running exporter and monitoring stack." + tui.Reset)
	}
	fmt.Println(tui.Bold + "================================================================" + tui.Reset)
	tui.Pause()
}

func selectTargetAndHealth(docker *dockerapi.Client, cfg config.Config) (string, string) {
	containers, err := docker.ListApplicationContainers(context.Background(), cfg.ApplicationDirectory)
	if err != nil {
		tui.Warning("Cannot read application containers: " + err.Error())
		tui.Pause()
		return "", ""
	}
	if len(containers) == 0 {
		tui.Warning("No application containers are running.")
		tui.Info("Start application services: docker compose -f aut/microservices-demo/deploy/docker-compose/docker-compose.yml up -d")
		tui.Pause()
		return "", ""
	}
	options := make([]string, len(containers))
	for i, c := range containers {
		options[i] = fmt.Sprintf("%-20s %s", c.DisplayName(), c.Labels["com.docker.compose.service"])
	}
	choice := tui.Menu("Application services", options)
	if choice < 0 {
		return "", ""
	}
	target := containers[choice].DisplayName()
	healthURL := tui.AskDefault("Recovery endpoint (type skip to disable)", cfg.TargetHealthURL)
	if strings.EqualFold(healthURL, "skip") {
		healthURL = ""
	}
	return target, healthURL
}

func checkSystemStatus(cfg config.Config, docker *dockerapi.Client) {
	tui.Clear()
	tui.Banner()
	fmt.Println(tui.Bold + "System status" + tui.Reset)
	fmt.Println()

	client := http.Client{Timeout: 3 * time.Second}

	fmt.Print("Docker daemon (" + cfg.DockerSocket + ")... ")
	containers, err := docker.ListApplicationContainers(context.Background(), cfg.ApplicationDirectory)
	if err != nil {
		fmt.Println(tui.Red + "UNREACHABLE" + tui.Reset)
		fmt.Printf("  error: %v\n", err)
	} else {
		fmt.Printf("%sOK%s (%d application services running)\n", tui.Green, tui.Reset, len(containers))
		for _, c := range containers {
			name := c.ID
			if len(name) > 12 {
				name = name[:12]
			}
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
		if resp != nil {
			defer resp.Body.Close()
		}
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

	fmt.Println()
	checkMetricsConnection(client)

	tui.Pause()
}

func runFullSuite(docker *dockerapi.Client, ctrl *controller.Controller, cfg config.Config) {
	tui.Clear()
	tui.Banner()
	fmt.Println(tui.Bold + "Running the full test suite" + tui.Reset)
	fmt.Println()

	target, healthURL := selectTargetAndHealth(docker, cfg)
	if target == "" {
		return
	}
	fmt.Println()
	failures := 0
	run := func(label string, work func() error) {
		if err := tui.Spin(label, work); err != nil {
			failures++
		}
	}

	run("container-pause attack (10s)", func() error {
		return ctrl.RunAttackWithHealthURL(context.Background(), "container-pause", target, plugins.Params{"duration_s": "10"}, healthURL)
	})

	run("latency attack (300ms, 15s)", func() error {
		return ctrl.RunAttackWithHealthURL(context.Background(), "latency", target, plugins.Params{
			"delay_ms": "300", "jitter_ms": "20", "duration_s": "15",
		}, healthURL)
	})

	run("cpu attack (cgroup throttle, 15s)", func() error {
		return ctrl.RunAttackWithHealthURL(context.Background(), "cpu", target, plugins.Params{
			"quota_pct": "10", "duration_s": "15",
		}, healthURL)
	})

	var latResult threshold.Result
	if healthURL != "" {
		run("resilience threshold test (latency)", func() error {
			net := network.NewManager(docker, "eth0")
			var err error
			latResult, err = threshold.RunLatencyThreshold(context.Background(), net, target, healthURL)
			return err
		})
		if latResult.BreakingPoint > 0 {
			hist := resilience.NewHistory("resilience-history.json")
			hist.Append("interactive-full-suite", []threshold.Result{latResult})
		}

	} else {
		tui.Warning("Threshold test skipped: a health URL is required.")
	}

	var loadResult loadtest.Result
	if healthURL != "" {
		run("load test (concurrent users)", func() error {
			var err error
			loadResult, err = loadtest.Run(context.Background(), docker, target, healthURL)
			return err
		})
	}

	fmt.Println()
	if failures > 0 {
		tui.Warning(fmt.Sprintf("Suite finished with %d failed checks. Review the errors above.", failures))
	} else {
		tui.Success("All executed checks passed.")
	}
	fmt.Println()
	fmt.Println(tui.Bold + "Summary:" + tui.Reset)
	fmt.Printf("  latency breaking point:     %d ms\n", latResult.BreakingPoint)
	if healthURL != "" {
		fmt.Printf("  max concurrent users:       %d\n", loadResult.MaxConcurrentUsers)
		fmt.Printf("  CPU usage at max load:      %.1f%%\n", loadResult.CPUPercent)
		fmt.Printf("  memory usage at max load:   %.1f MB\n", loadResult.MemUsedMB)
	}
	fmt.Println()
	fmt.Println(tui.Red + "View full results and trends in Grafana: http://localhost:3001" + tui.Reset)

	tui.Pause()
}

func attackMenu(docker *dockerapi.Client, ctrl *controller.Controller, cfg config.Config) {
	tui.Clear()
	tui.Banner()
	names := plugins.List()
	choice := tui.Menu("Choose an attack", names)
	if choice < 0 {
		return
	}
	attackName := names[choice]
	target, healthURL := selectTargetAndHealth(docker, cfg)
	if target == "" {
		return
	}

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
	tui.Banner()
	choice := tui.Menu("Choose what to threshold-test", []string{"latency", "cpu"})
	if choice < 0 {
		return
	}
	target, healthURL := selectTargetAndHealth(docker, cfg)
	if healthURL == "" {
		tui.Warning("Threshold tests require a health URL.")
		tui.Pause()
		return
	}
	if target == "" {
		return
	}

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
	tui.Banner()
	target, defaultHealth := selectTargetAndHealth(docker, cfg)
	if target == "" {
		return
	}
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
	tui.Banner()
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
	tui.Banner()
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
	tui.Banner()
	fmt.Println(tui.Bold + "Registered attack plugins" + tui.Reset)
	for _, name := range plugins.List() {
		p, _ := plugins.Get(name)
		fmt.Printf("  %s%-16s%s %s\n", tui.Red, name, tui.Reset, p.Describe())
	}
	tui.Pause()
}

func startMetricsBackground(cfg config.Config, m *metrics.Collector) {
	tui.Clear()
	tui.Banner()
	if metricsRunning {
		fmt.Println(tui.Yellow + "metrics server is already running on " + cfg.MetricsAddr + tui.Reset)
		tui.Pause()
		return
	}

	if err := launchMetrics(cfg, m); err != nil {
		tui.Warning("Metrics server could not start: " + err.Error())
		tui.Pause()
		return
	}

	fmt.Println(tui.Green + "metrics server started in the background on " + cfg.MetricsAddr + tui.Reset)
	fmt.Println(tui.Gray + "it will keep running for the rest of this session - you can use every" + tui.Reset)
	fmt.Println(tui.Gray + "other menu option normally while it serves Prometheus in the background." + tui.Reset)
	fmt.Println()
	fmt.Println(tui.Red + "Prometheus scrape endpoint: http://localhost" + cfg.MetricsAddr + "/metrics" + tui.Reset)
	fmt.Println(tui.Red + "View the dashboard at http://localhost:3001" + tui.Reset)
	tui.Pause()
}

// Bind before reporting success so an occupied port is visible immediately.
func launchMetrics(cfg config.Config, m *metrics.Collector) error {
	listener, err := net.Listen("tcp", cfg.MetricsAddr)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", m.Handler())
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	metricsRunning = true
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("\nMetrics server stopped: %v\n", err)
		}
	}()
	return nil
}

func checkMetricsConnection(client http.Client) {
	fmt.Print("Metrics reaching Prometheus... ")
	resp, err := client.Get("http://localhost:9091/api/v1/targets")
	if err != nil {
		tui.Warning("Cannot reach Prometheus. Start the monitoring stack.")
		return
	}
	defer resp.Body.Close()
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			Targets []struct {
				Labels    map[string]string `json:"labels"`
				Health    string            `json:"health"`
				LastError string            `json:"lastError"`
			} `json:"activeTargets"`
		} `json:"data"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&payload) != nil || payload.Status != "success" {
		tui.Warning("Could not read metrics connection status.")
		return
	}
	for _, target := range payload.Data.Targets {
		if target.Labels["job"] != "gremlin" {
			continue
		}
		if target.Health == "up" {
			tui.Success("Connected. Grafana can read your test results.")
			return
		}
		tui.Warning("Disconnected: " + target.LastError)
		tui.Info("Restart monitoring: docker compose -f monitoring/docker-compose.yml up -d --build")
		return
	}
	tui.Warning("No Gremlin metrics source configured in Prometheus.")
}

func printApplicationWorkspace(cfg config.Config, docker *dockerapi.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	containers, err := docker.ListApplicationContainers(ctx, cfg.ApplicationDirectory)
	status := fmt.Sprintf("%d services online", len(containers))
	if err != nil {
		status = "Docker unavailable"
	}
	fmt.Printf("  %sAPPLICATION%s  %-22s   %sMODE%s  interactive\n", tui.Gray, tui.Reset, status, tui.Gray, tui.Reset)
	fmt.Println(tui.Gray + "  " + cfg.ApplicationDirectory + tui.Reset)
}
