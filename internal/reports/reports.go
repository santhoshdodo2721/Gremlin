package reports

import (
"encoding/json"
"fmt"
"os"
"sync"
"time"
)

type Report struct {
ID         string    `json:"id"`
Attack     string    `json:"attack"`
Target     string    `json:"target"`
StartedAt  time.Time `json:"started_at"`
DurationMs int64     `json:"duration_ms"`
RecoveryMs int64     `json:"recovery_ms"`
Success    bool      `json:"success"`
Message    string    `json:"message"`
}

type Store struct {
path string
mu   sync.Mutex
}

func NewStore(path string) *Store {
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
return os.WriteFile(s.path, data, 0644)
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
fmt.Printf("%-12s %-16s %-20s %-10s %-10s %-8s %s\n", "ID", "ATTACK", "TARGET", "DURATION", "RECOVERY", "OK", "MESSAGE")
for _, r := range reports {
fmt.Printf("%-12s %-16s %-20s %-10s %-10s %-8v %s\n",
r.ID, r.Attack, r.Target,
fmt.Sprintf("%dms", r.DurationMs),
fmt.Sprintf("%dms", r.RecoveryMs),
r.Success, r.Message)
}
}
