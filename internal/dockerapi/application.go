package dockerapi

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
)

// ApplicationContainers uses Compose ownership metadata, never container-name guesses.
// Monitoring and unrelated Compose projects therefore cannot enter the target menu.
func ApplicationContainers(containers []Container, directory string) []Container {
	if strings.TrimSpace(directory) == "" {
		return nil
	}
	directory, err := filepath.Abs(directory)
	if err != nil || directory == "" {
		return nil
	}
	directory = filepath.Clean(directory)
	var selected []Container
	for _, container := range containers {
		workingDir := container.Labels["com.docker.compose.project.working_dir"]
		belongs := workingDir != "" && filepath.Clean(workingDir) == directory
		if workingDir == "" {
			for _, file := range strings.Split(container.Labels["com.docker.compose.project.config_files"], ",") {
				if filepath.IsAbs(file) && filepath.Dir(filepath.Clean(file)) == directory {
					belongs = true
					break
				}
			}
		}
		if belongs {
			selected = append(selected, container)
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].DisplayName() < selected[j].DisplayName() })
	return selected
}

func (c Container) DisplayName() string {
	if len(c.Names) > 0 {
		return strings.TrimPrefix(c.Names[0], "/")
	}
	if len(c.ID) > 12 {
		return c.ID[:12]
	}
	return c.ID
}

func (c *Client) ListApplicationContainers(ctx context.Context, directory string) ([]Container, error) {
	containers, err := c.ListContainers(ctx)
	if err != nil {
		return nil, err
	}
	return ApplicationContainers(containers, directory), nil
}

func (c Container) Matches(target string) bool {
	if target == c.ID {
		return true
	}
	if len(target) >= 12 && strings.HasPrefix(c.ID, target) {
		return true
	}
	for _, name := range c.Names {
		if strings.TrimPrefix(name, "/") == strings.TrimPrefix(target, "/") {
			return true
		}
	}
	return false
}
