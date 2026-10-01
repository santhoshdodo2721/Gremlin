package reports

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"text/tabwriter"
	"time"
)

type Report struct {
	ID             string    `json:"id"`
	Attack         string    `json:"attack"`
	Target         string    `json:"target"`
	StartedAt      time.Time `json:"started_at"`
	DurationMs     int64     `json:"duration_ms"`
	RecoveryMs     int64     `json:"recovery_ms"`
	Success        bool      `json:"success"`
	Message        string    `json:"message"`
	RecoveryStatus string    `json:"recovery_status,omitempty"`
}

// RecoveryOutcome keeps legacy zero values ambiguous: they may be skipped checks.
func (r Report) RecoveryOutcome() string {
	switch r.RecoveryStatus {
	case "verified", "skipped", "unverified":
		return r.RecoveryStatus
	}
	if r.RecoveryMs > 0 {
		return "verified"
	}
	return "unknown"
}

type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if _, errParent := os.Stat("../" + path); errParent == nil {
			path = "../" + path
		}
	}
	return &Store{path: path}
}

func (s *Store) load() ([]Report, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []Report{}, nil
	}
	if err != nil {
		return nil, err
	}
	var reports []Report
	if len(data) == 0 {
		return []Report{}, nil
	}
	if err := json.Unmarshal(data, &reports); err != nil {
		return nil, err
	}
	return reports, nil
}

func (s *Store) Save(r Report) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	reports, err := s.load()
	if err != nil {
		return err
	}
	reports = append(reports, r)

	data, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(s.path), ".gremlin-reports-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}

func (s *Store) List() ([]Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	reports, err := s.load()
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(reports)-1; i < j; i, j = i+1, j-1 {
		reports[i], reports[j] = reports[j], reports[i]
	}
	return reports, nil
}

func Print(reports []Report) {
	if len(reports) == 0 {
		fmt.Println("No attacks recorded yet. Run gremlin menu to start a guided test.")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 3, ' ', 0)
	fmt.Fprintln(w, "STARTED\tATTACK\tTARGET\tDURATION\tRECOVERY\tRESULT\tMESSAGE")
	for _, r := range reports {
		recovery := fmt.Sprintf("%dms", r.RecoveryMs)
		if r.RecoveryOutcome() != "verified" {
			recovery = r.RecoveryOutcome()
		}
		result := "FAIL"
		if r.Success {
			result = "PASS"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%dms\t%s\t%s\t%s\n", r.StartedAt.Format("Jan 02 15:04:05"), r.Attack, r.Target, r.DurationMs, recovery, result, r.Message)
	}
	w.Flush()
}
