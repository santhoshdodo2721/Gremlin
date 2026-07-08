package config

import (
"encoding/json"
"os"
)

type Config struct {
DockerSocket    string `json:"docker_socket"`
TargetHealthURL string `json:"target_health_url"`
ReportsFile     string `json:"reports_file"`
MetricsAddr     string `json:"metrics_addr"`
}

func Default() Config {
return Config{
DockerSocket:    "/var/run/docker.sock",
TargetHealthURL: "http://localhost:8080/health",
ReportsFile:     "gremlin-reports.json",
MetricsAddr:     ":9090",
}
}

func Load(path string) (Config, error) {
cfg := Default()
if path == "" {
return cfg, nil
}
data, err := os.ReadFile(path)
if err != nil {
if os.IsNotExist(err) {
return cfg, nil
}
return cfg, err
}
if err := json.Unmarshal(data, &cfg); err != nil {
return cfg, err
}
return cfg, nil
}

type ScheduleJob struct {
Attack       string            `json:"attack"`
Target       string            `json:"target"`
EverySeconds int               `json:"every_seconds"`
Params       map[string]string `json:"params"`
}

type Schedule struct {
Jobs []ScheduleJob `json:"jobs"`
}

func LoadSchedule(path string) (Schedule, error) {
var sched Schedule
data, err := os.ReadFile(path)
if err != nil {
if os.IsNotExist(err) {
return sched, nil
}
return sched, err
}
if err := json.Unmarshal(data, &sched); err != nil {
return sched, err
}
return sched, nil
}
