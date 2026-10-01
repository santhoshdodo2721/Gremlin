package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	DockerSocket         string `json:"docker_socket"`
	TargetHealthURL      string `json:"target_health_url"`
	ReportsFile          string `json:"reports_file"`
	MetricsAddr          string `json:"metrics_addr"`
	ApplicationDirectory string `json:"application_directory"`
}

func Default() Config {
	return Config{
		DockerSocket:         "/var/run/docker.sock",
		TargetHealthURL:      "http://localhost/",
		ReportsFile:          "gremlin-reports.json",
		MetricsAddr:          ":9090",
		ApplicationDirectory: "aut/microservices-demo/deploy/docker-compose",
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	base := "."
	if path != "" {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			alternate := filepath.Join("..", path)
			if other, otherErr := os.ReadFile(alternate); otherErr == nil {
				data, err, path = other, nil, alternate
			}
		}
		if err != nil && !os.IsNotExist(err) {
			return cfg, err
		}
		if err == nil {
			if err := json.Unmarshal(data, &cfg); err != nil {
				return cfg, err
			}
			base = filepath.Dir(path)
		}
	}
	if cfg.ApplicationDirectory == "" {
		return cfg, fmt.Errorf("application_directory cannot be empty")
	}
	if !filepath.IsAbs(cfg.ApplicationDirectory) {
		cfg.ApplicationDirectory = filepath.Join(base, cfg.ApplicationDirectory)
	}
	directory, err := filepath.Abs(cfg.ApplicationDirectory)
	if err != nil {
		return cfg, err
	}
	cfg.ApplicationDirectory = filepath.Clean(directory)
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
	if err != nil && os.IsNotExist(err) {
		data, err = os.ReadFile("../" + path)
	}
	if err != nil {
		if os.IsNotExist(err) {
			return sched, nil
		}
		return sched, err
	}
	if err := json.Unmarshal(data, &sched); err != nil {
		return sched, err
	}
	for i, job := range sched.Jobs {
		if job.Attack == "" || job.Target == "" || job.EverySeconds <= 0 {
			return sched, fmt.Errorf("schedule job %d requires attack, target, and positive every_seconds", i+1)
		}
	}
	return sched, nil
}
