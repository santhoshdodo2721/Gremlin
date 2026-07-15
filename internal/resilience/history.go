package resilience

import (
"encoding/json"
"fmt"
"os"
"sync"

"gremlin-in-a-box/internal/threshold"
)

type Entry struct {
CommitSHA string              `json:"commit_sha"`
Results   []threshold.Result  `json:"results"`
}

type History struct {
path string
mu   sync.Mutex
}

func NewHistory(path string) *History {
return &History{path: path}
}

func (h *History) load() ([]Entry, error) {
data, err := os.ReadFile(h.path)
if os.IsNotExist(err) {
return []Entry{}, nil
}
if err != nil {
return nil, err
}
if len(data) == 0 {
return []Entry{}, nil
}
var entries []Entry
if err := json.Unmarshal(data, &entries); err != nil {
return nil, err
}
return entries, nil
}

// Append adds a new entry (one CI run/local run) with all threshold results
// collected during that run.
func (h *History) Append(commitSHA string, results []threshold.Result) error {
h.mu.Lock()
defer h.mu.Unlock()

entries, err := h.load()
if err != nil {
return err
}
entries = append(entries, Entry{CommitSHA: commitSHA, Results: results})

data, err := json.MarshalIndent(entries, "", "  ")
if err != nil {
return err
}
return os.WriteFile(h.path, data, 0644)
}

// Regression describes one attack type whose breaking point got worse.
type Regression struct {
Attack   string
Previous int
Current  int
DropPct  float64
}

// CheckRegressions compares the most recent entry against the one before
// it. tolerancePct is how much drop is acceptable before it counts as a
// regression (e.g. 20 means a >20% drop fails).
func (h *History) CheckRegressions(tolerancePct float64) ([]Regression, error) {
h.mu.Lock()
defer h.mu.Unlock()

entries, err := h.load()
if err != nil {
return nil, err
}
if len(entries) < 2 {
return nil, nil
}

prev := entries[len(entries)-2]
curr := entries[len(entries)-1]

prevByAttack := map[string]int{}
for _, r := range prev.Results {
prevByAttack[r.Attack] = r.BreakingPoint
}

var regressions []Regression
for _, r := range curr.Results {
old, ok := prevByAttack[r.Attack]
if !ok || old == 0 {
continue
}
dropPct := 100.0 * float64(old-r.BreakingPoint) / float64(old)
if dropPct > tolerancePct {
regressions = append(regressions, Regression{
Attack:   r.Attack,
Previous: old,
Current:  r.BreakingPoint,
DropPct:  dropPct,
})
}
}
return regressions, nil
}

func PrintRegressions(regs []Regression) {
for _, r := range regs {
fmt.Printf("REGRESSION: %s breaking point dropped from %d to %d (%.1f%% worse)\n",
r.Attack, r.Previous, r.Current, r.DropPct)
}
}
