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
