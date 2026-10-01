package plugins

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ValidateParams rejects malformed input before modifying a container.
func ValidateParams(attack, target string, params Params) error {
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("target container is required")
	}
	bounds := map[string][2]int{
		"duration_s": {1, 86400}, "delay_ms": {0, 3600000},
		"jitter_ms": {0, 3600000}, "quota_pct": {1, 100}, "workers": {1, 1024},
	}
	for key, limit := range bounds {
		if value := params[key]; value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n < limit[0] || n > limit[1] {
				return fmt.Errorf("%s must be an integer between %d and %d", key, limit[0], limit[1])
			}
		}
	}
	if value := params["percent"]; value != "" {
		n, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 100 {
			return fmt.Errorf("percent must be a number between 0 and 100")
		}
	}
	if attack == "cpu" {
		switch params["method"] {
		case "", "auto", "stress", "throttle":
		default:
			return fmt.Errorf("CPU method must be auto, stress, or throttle")
		}
	}
	if value := params["restart"]; value != "" && value != "true" && value != "false" {
		return fmt.Errorf("restart must be true or false")
	}
	return nil
}
