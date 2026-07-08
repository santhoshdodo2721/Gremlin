package main

import (
"context"
"flag"
"fmt"
"os"

"gremlin-in-a-box/internal/config"
"gremlin-in-a-box/internal/controller"
"gremlin-in-a-box/internal/dockerapi"
"gremlin-in-a-box/internal/logger"
"gremlin-in-a-box/internal/metrics"
"gremlin-in-a-box/internal/plugins"
"gremlin-in-a-box/internal/reports"
)

func main() {
if len(os.Args) < 2 {
printUsage()
os.Exit(1)
}

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

switch os.Args[1] {
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

default:
printUsage()
os.Exit(1)
}
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
fmt.Println("  gremlin version")
fmt.Println("  gremlin status")
fmt.Println("  gremlin attack --name <attack> --target <container> [flags]")
fmt.Println("  gremlin report")
fmt.Println("  gremlin serve-metrics")
}
