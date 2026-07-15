package main

import (
"context"
"flag"
"fmt"
"os"

"gremlin-in-a-box/internal/dockerapi"
"gremlin-in-a-box/internal/network"
"gremlin-in-a-box/internal/resilience"
"gremlin-in-a-box/internal/threshold"
)

func runThresholdCmd(docker *dockerapi.Client, healthURL, historyFile string) {
fs := flag.NewFlagSet("threshold", flag.ExitOnError)
attack := fs.String("attack", "", "latency or cpu")
target := fs.String("target", "", "container to test")
commit := fs.String("commit", "local", "commit SHA to tag this run with")
fs.Parse(os.Args[2:])

if *attack == "" || *target == "" {
fmt.Println("usage: gremlin threshold --attack latency|cpu --target <container>")
os.Exit(1)
}

ctx := context.Background()
var result threshold.Result
var err error

switch *attack {
case "latency":
net := network.NewManager(docker, "eth0")
result, err = threshold.RunLatencyThreshold(ctx, net, *target, healthURL)
case "cpu":
result, err = threshold.RunCPUThreshold(ctx, docker, *target, healthURL)
default:
fmt.Println("unknown attack type:", *attack)
os.Exit(1)
}

if err != nil {
fmt.Fprintln(os.Stderr, "threshold test error:", err)
os.Exit(1)
}

fmt.Printf("breaking point for %s: %d %s\n", result.Attack, result.BreakingPoint, result.Unit)

hist := resilience.NewHistory(historyFile)
if err := hist.Append(*commit, []threshold.Result{result}); err != nil {
fmt.Fprintln(os.Stderr, "failed to save history:", err)
}
}

func runThresholdCheckCmd(historyFile string) {
fs := flag.NewFlagSet("threshold-check", flag.ExitOnError)
tolerance := fs.Float64("tolerance-pct", 20.0, "allowed percent drop before failing")
fs.Parse(os.Args[2:])

hist := resilience.NewHistory(historyFile)
regs, err := hist.CheckRegressions(*tolerance)
if err != nil {
fmt.Fprintln(os.Stderr, "error checking regressions:", err)
os.Exit(1)
}
if len(regs) == 0 {
fmt.Println("no resilience regressions detected")
return
}
resilience.PrintRegressions(regs)
os.Exit(1)
}
