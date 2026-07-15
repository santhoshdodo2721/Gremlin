package main

import (
"context"
"encoding/json"
"flag"
"fmt"
"os"

"gremlin-in-a-box/internal/dockerapi"
"gremlin-in-a-box/internal/loadtest"
)

func runLoadtestCmd(docker *dockerapi.Client) {
fs := flag.NewFlagSet("loadtest", flag.ExitOnError)
target := fs.String("target", "", "container to load test")
url := fs.String("url", "", "URL to hammer, e.g. http://localhost:8080/health")
fs.Parse(os.Args[2:])

if *target == "" || *url == "" {
fmt.Println("usage: gremlin loadtest --target <container> --url <http endpoint>")
os.Exit(1)
}

fmt.Println("ramping up concurrent users against", *url, "...")
result, err := loadtest.Run(context.Background(), docker, *target, *url)
if err != nil {
fmt.Fprintln(os.Stderr, "load test error:", err)
os.Exit(1)
}

fmt.Printf("max concurrent users tolerated: %d\n", result.MaxConcurrentUsers)
fmt.Printf("requests per second at that level: %.1f\n", result.RequestsPerSecond)
fmt.Printf("average latency: %d ms\n", result.AvgLatencyMs)
fmt.Printf("error rate: %.1f%%\n", result.ErrorRatePercent)
fmt.Printf("CPU usage: %.1f%%\n", result.CPUPercent)
fmt.Printf("memory usage: %.1f MB\n", result.MemUsedMB)

data, _ := json.MarshalIndent(result, "", "  ")
os.WriteFile("load-test-result.json", data, 0644)
fmt.Println("saved to load-test-result.json")
}
