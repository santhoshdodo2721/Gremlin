package main

import "testing"

func TestContinuousPlanRestoresContainers(t *testing.T) {
	plan, err := continuousPlan([]string{"latency", "packet-loss", "corruption", "cpu", "container-pause", "container-kill"}, 5)
	if err != nil || len(plan) != 6 {
		t.Fatalf("plan=%v err=%v", plan, err)
	}
	if plan[3].Params["method"] != "throttle" || plan[5].Params["restart"] != "true" {
		t.Fatal("continuous plan must restore container state")
	}
	for _, names := range [][]string{{"unknown"}, {""}, nil} {
		if _, err := continuousPlan(names, 5); err == nil {
			t.Fatalf("accepted %v", names)
		}
	}
}
