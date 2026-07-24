package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gremlin-in-a-box/internal/config"
	"gremlin-in-a-box/internal/controller"
	"gremlin-in-a-box/internal/dockerapi"
	"gremlin-in-a-box/internal/logger"
	"gremlin-in-a-box/internal/metrics"
	"gremlin-in-a-box/internal/plugins"
	"gremlin-in-a-box/internal/reports"
	"gremlin-in-a-box/internal/scheduler"
)

func main() {
	cfg, err := config.Load("config.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}

	log := logger.New(os.Getenv("GREMLIN_DEBUG") == "true")
	docker := dockerapi.New(cfg.DockerSocket)
	plugins.RegisterDefaults(docker)

	store := reports.NewStore(cfg.ReportsFile)
	m := metrics.NewCollector(store)
	ctrl := controller.New(log, store, cfg.TargetHealthURL)

	if len(os.Args) < 2 {
		runInteractiveMenu(cfg, docker, ctrl, store, m)
		return
	}

	switch os.Args[1] {
	case "menu":
		runInteractiveMenu(cfg, docker, ctrl, store, m)

	case "version":
		fmt.Println("gremlin v0.1.0")

	case "status":
		fmt.Println("Registered attack plugins:")
		for _, name := range plugins.List() {
			p, _ := plugins.Get(name)
			fmt.Printf("  %-16s %s\n", name, p.Describe())
		}

	case "attack":
		runAttackCmd(ctrl)

	case "report":
		list, err := store.List()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error reading reports:", err)
			os.Exit(1)
		}
		reports.Print(list)

	case "serve-metrics":
		fmt.Println("serving metrics on", cfg.MetricsAddr)
		if err := m.Serve(cfg.MetricsAddr); err != nil {
			fmt.Fprintln(os.Stderr, "metrics server error:", err)
			os.Exit(1)
		}

	case "threshold":
		runThresholdCmd(docker, cfg.TargetHealthURL, "resilience-history.json")

	case "threshold-check":
		runThresholdCheckCmd("resilience-history.json")

	case "loadtest":
		runLoadtestCmd(docker)

	case "schedule":
		runScheduleCmd(log, ctrl)

	default:
		printUsage()
		os.Exit(1)
	}
}

func runScheduleCmd(log *slog.Logger, ctrl *controller.Controller) {
	fs := flag.NewFlagSet("schedule", flag.ExitOnError)
	scheduleFile := fs.String("file", "schedule.json", "path to schedule config file")
	fs.Parse(os.Args[2:])

	sched, err := config.LoadSchedule(*scheduleFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "schedule error:", err)
		os.Exit(1)
	}
	if len(sched.Jobs) == 0 {
		fmt.Println("no jobs found in", *scheduleFile)
		os.Exit(1)
	}

	var jobs []scheduler.Job
	for _, j := range sched.Jobs {
		jobs = append(jobs, scheduler.Job{
			Attack: j.Attack,
			Target: j.Target,
			Params: plugins.Params(j.Params),
			Every:  time.Duration(j.EverySeconds) * time.Second,
		})
		fmt.Printf("scheduled: %-16s target=%-12s every=%ds\n", j.Attack, j.Target, j.EverySeconds)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("scheduler running - press Ctrl+C to stop")
	scheduler.Run(ctx, log, ctrl, jobs)
}

func runAttackCmd(ctrl *controller.Controller) {
	fs := flag.NewFlagSet("attack", flag.ExitOnError)
	attackName := fs.String("name", "", "attack plugin to run")
	target := fs.String("target", "", "container ID or name to attack")
	delayMs := fs.String("delay-ms", "", "latency plugin: delay in ms")
	jitterMs := fs.String("jitter-ms", "", "latency plugin: jitter in ms")
	durationS := fs.String("duration-s", "", "attack duration in seconds")
	workers := fs.String("workers", "", "cpu plugin: number of stress workers")
	restart := fs.String("restart", "", "container-kill plugin: restart after stopping")
	fs.Parse(os.Args[2:])

	if *attackName == "" || *target == "" {
		fmt.Println("usage: gremlin attack --name <attack> --target <container> [flags]")
		os.Exit(1)
	}

	params := plugins.Params{}
	if *delayMs != "" {
		params["delay_ms"] = *delayMs
	}
	if *jitterMs != "" {
		params["jitter_ms"] = *jitterMs
	}
	if *durationS != "" {
		params["duration_s"] = *durationS
	}
	if *workers != "" {
		params["workers"] = *workers
	}
	if *restart != "" {
		params["restart"] = *restart
	}

	if err := ctrl.RunAttack(context.Background(), *attackName, *target, params); err != nil {
		fmt.Fprintln(os.Stderr, "attack error:", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("gremlin - chaos engineering CLI")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  gremlin                (launches interactive menu)")
	fmt.Println("  gremlin menu")
	fmt.Println("  gremlin version")
	fmt.Println("  gremlin status")
	fmt.Println("  gremlin attack --name <attack> --target <container> [flags]")
	fmt.Println("  gremlin report")
	fmt.Println("  gremlin serve-metrics")
	fmt.Println("  gremlin schedule --file schedule.json")
	fmt.Println("  gremlin threshold --attack latency|cpu --target <container>")
	fmt.Println("  gremlin threshold-check")
	fmt.Println("  gremlin loadtest --target <container> --url <http endpoint>")
}
