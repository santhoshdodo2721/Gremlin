package metrics

import (
"fmt"
"net"
"net/http"

"gremlin-in-a-box/internal/reports"
)

type Collector struct {
store *reports.Store
}

func NewCollector(store *reports.Store) *Collector {
return &Collector{store: store}
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
type typeStats struct {
total, failed             int
sumRecovery, sumDuration  int64
}
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
}

w.Header().Set("Content-Type", "text/plain; version=0.0.4")

fmt.Fprintf(w, "gremlin_attacks_total %d\n", total)
fmt.Fprintf(w, "gremlin_attacks_failed_total %d\n", failed)

successRate := 100.0
if total > 0 {
successRate = 100.0 * float64(total-failed) / float64(total)
}
fmt.Fprintf(w, "gremlin_success_rate_percent %.2f\n", successRate)
fmt.Fprintf(w, "gremlin_last_recovery_ms %d\n", lastRecovery)
fmt.Fprintf(w, "gremlin_last_duration_ms %d\n", lastDuration)

for name, st := range byType {
fmt.Fprintf(w, "gremlin_attacks_by_type_total{attack=\"%s\"} %d\n", name, st.total)
}
for name, st := range byType {
fmt.Fprintf(w, "gremlin_attacks_failed_by_type_total{attack=\"%s\"} %d\n", name, st.failed)
}
for name, st := range byType {
avg := int64(0)
if st.total > 0 {
avg = st.sumRecovery / int64(st.total)
}
fmt.Fprintf(w, "gremlin_avg_recovery_ms_by_type{attack=\"%s\"} %d\n", name, avg)
}
for name, st := range byType {
avg := int64(0)
if st.total > 0 {
avg = st.sumDuration / int64(st.total)
}
fmt.Fprintf(w, "gremlin_avg_duration_ms_by_type{attack=\"%s\"} %d\n", name, avg)
}
})
}

func (c *Collector) TryListen(addr string) (net.Listener, error) {
return net.Listen("tcp", addr)
}

func (c *Collector) ServeOnListener(ln net.Listener) error {
mux := http.NewServeMux()
mux.Handle("/metrics", c.Handler())
return http.Serve(ln, mux)
}

func (c *Collector) Serve(addr string) error {
mux := http.NewServeMux()
mux.Handle("/metrics", c.Handler())
return http.ListenAndServe(addr, mux)
}
