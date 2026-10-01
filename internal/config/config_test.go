package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInvalidSchedule(t *testing.T) {
	for _, data := range []string{`{"jobs":[{"attack":"cpu","target":"api","every_seconds":0}]}`, `{"jobs":[{"attack":"cpu","every_seconds":10}]}`} {
		path := filepath.Join(t.TempDir(), "schedule.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSchedule(path); err == nil {
			t.Fatal("accepted invalid job")
		}
	}
}
