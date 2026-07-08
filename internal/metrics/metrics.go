package metrics

import (
"fmt"
"net/http"

"gremlin-in-a-box/internal/reports"
)

type Collector struct {
store *reports.Store
}

func NewCollector(store *reports.Store) *Collector {
return &Collector{store: store}
}

type typeStats struct {
total        int
failed       int
sumRecovery  int64
sumDuration  int64
lastRecovery int64
lastDuration int64
}

func (c *Collector) Handler() http.Handler {
return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
list, err := c.store.List()
if err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}

var total, failed int
var lastDuration, lastRecovery int64
byType := map[string]*typeStats{}

for i, rep := range list {
total++
if !rep.Success {
failed++
}
if i == 0 {
lastDuration = rep.DurationMs
lastRecovery = rep.RecoveryMs
}
st, ok := byType[rep.Attack]
if !ok {
st = &typeStats{}
byType[rep.Attack] = st
}
st.total++
if !rep.Success {
st.failed++
}
st.sumRecovery += rep.RecoveryMs
st.sumDuration += rep.DurationMs
if i == 0 || st.total == 1 {
st.lastRecovery = rep.RecoveryMs
st.lastDuration = rep.DurationMs
}
}

w.Header().Set("Content-Type", "text/plain; version=0.0.4")

fmt.Fprintf(w, "# HELP gremlin_attacks_total Total chaos attacks run\n")
fmt.Fprintf(w, "# TYPE gremlin_attacks_total counter\n")
fmt.Fprintf(w, "gremlin_attacks_total %d\n", total)

fmt.Fprintf(w, "# HELP gremlin_attacks_failed_total Attacks that errored\n")
fmt.Fprintf(w, "# TYPE gremlin_attacks_failed_total counter\n")
fmt.Fprintf(w, "gremlin_attacks_failed_total %d\n", failed)

successRate := 100.0
if total > 0 {
successRate = 100.0 * float64(total-failed) / float64(total)
}
fmt.Fprintf(w, "# HELP gremlin_success_rate_percent Percentage of attacks that succeeded\n")
fmt.Fprintf(w, "# TYPE gremlin_success_rate_percent gauge\n")
fmt.Fprintf(w, "gremlin_success_rate_percent %.2f\n", successRate)

fmt.Fprintf(w, "# HELP gremlin_last_recovery_ms Recovery time of the most recent attack\n")
fmt.Fprintf(w, "# TYPE gremlin_last_recovery_ms gauge\n")
fmt.Fprintf(w, "gremlin_last_recovery_ms %d\n", lastRecovery)

fmt.Fprintf(w, "# HELP gremlin_last_duration_ms Duration of the most recent attack\n")
fmt.Fprintf(w, "# TYPE gremlin_last_duration_ms gauge\n")
fmt.Fprintf(w, "gremlin_last_duration_ms %d\n", lastDuration)

fmt.Fprintf(w, "# HELP gremlin_attacks_by_type_total Attacks run, broken down by attack type\n")
fmt.Fprintf(w, "# TYPE gremlin_attacks_by_type_total counter\n")
for name, st := range byType {
fmt.Fprintf(w, "gremlin_attacks_by_type_total{attack=\"%s\"} %d\n", name, st.total)
}

fmt.Fprintf(w, "# HELP gremlin_attacks_failed_by_type_total Failed attacks, broken down by attack type\n")
fmt.Fprintf(w, "# TYPE gremlin_attacks_failed_by_type_total counter\n")
for name, st := range byType {
fmt.Fprintf(w, "gremlin_attacks_failed_by_type_total{attack=\"%s\"} %d\n", name, st.failed)
}

fmt.Fprintf(w, "# HELP gremlin_avg_recovery_ms_by_type Average recovery time in ms, by attack type\n")
fmt.Fprintf(w, "# TYPE gremlin_avg_recovery_ms_by_type gauge\n")
for name, st := range byType {
avg := int64(0)
if st.total > 0 {
avg = st.sumRecovery / int64(st.total)
}
fmt.Fprintf(w, "gremlin_avg_recovery_ms_by_type{attack=\"%s\"} %d\n", name, avg)
}

fmt.Fprintf(w, "# HELP gremlin_avg_duration_ms_by_type Average attack duration in ms, by attack type\n")
fmt.Fprintf(w, "# TYPE gremlin_avg_duration_ms_by_type gauge\n")
for name, st := range byType {
avg := int64(0)
if st.total > 0 {
avg = st.sumDuration / int64(st.total)
}
fmt.Fprintf(w, "gremlin_avg_duration_ms_by_type{attack=\"%s\"} %d\n", name, avg)
}
})
}

func (c *Collector) Serve(addr string) error {
mux := http.NewServeMux()
mux.Handle("/metrics", c.Handler())
return http.ListenAndServe(addr, mux)
}
