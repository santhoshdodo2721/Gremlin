package metrics

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"gremlin-in-a-box/internal/reports"
)

type Collector struct{ store *reports.Store }

func NewCollector(store *reports.Store) *Collector { return &Collector{store: store} }

type typeStats struct {
	total, failed, verified int
	recovery, duration      int64
}

func (c *Collector) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		list, err := c.store.List()
		if err != nil {
			http.Error(w, "Unable to read saved reports", http.StatusInternalServerError)
			return
		}
		sort.SliceStable(list, func(i, j int) bool { return list[i].StartedAt.After(list[j].StartedAt) })
		byType := map[string]*typeStats{}
		failed, verified, skipped, unknown := 0, 0, 0, 0
		lastDuration, lastRecovery, lastTimestamp := math.NaN(), math.NaN(), math.NaN()
		lastStatus := float64(3)
		for i, rep := range list {
			outcome := rep.RecoveryOutcome()
			st := byType[rep.Attack]
			if st == nil {
				st = &typeStats{}
				byType[rep.Attack] = st
			}
			st.total++
			st.duration += rep.DurationMs
			if !rep.Success {
				failed++
				st.failed++
			}
			switch outcome {
			case "verified":
				if rep.RecoveryMs >= 0 {
					verified++
					st.verified++
					st.recovery += rep.RecoveryMs
				}
			case "skipped":
				skipped++
			case "unknown":
				unknown++
			}
			if i == 0 {
				lastDuration = float64(rep.DurationMs)
				lastTimestamp = float64(rep.StartedAt.Unix())
				switch outcome {
				case "verified":
					if rep.RecoveryMs >= 0 {
						lastRecovery = float64(rep.RecoveryMs)
						lastStatus = 1
					}
				case "skipped":
					lastStatus = 2
				case "unverified":
					lastStatus = 0
				}
			}
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Header().Set("Cache-Control", "no-store")
		emit := func(name, help, kind string, value float64) {
			fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n%s %g\n", name, help, name, kind, name, value)
		}
		total := len(list)
		passRate, coverage := math.NaN(), math.NaN()
		if total > 0 {
			passRate = 100 * float64(total-failed) / float64(total)
			coverage = 100 * float64(verified) / float64(total)
		}
		emit("gremlin_attacks_total", "Total saved attack reports", "counter", float64(total))
		emit("gremlin_attacks_failed_total", "Saved reports marked unsuccessful", "counter", float64(failed))
		emit("gremlin_success_rate_percent", "Percentage of reports marked successful; historical flags are preserved", "gauge", passRate)
		emit("gremlin_last_recovery_ms", "Latest recovery time; NaN when not verified", "gauge", lastRecovery)
		emit("gremlin_last_duration_ms", "Duration of the latest saved attack", "gauge", lastDuration)
		emit("gremlin_last_report_timestamp_seconds", "Start timestamp of latest saved report; not scrape time", "gauge", lastTimestamp)
		emit("gremlin_recovery_verified_total", "Reports with a verified nonnegative recovery measurement", "gauge", float64(verified))
		emit("gremlin_recovery_unverified_total", "Reports without a verified recovery measurement, including skipped and legacy records", "gauge", float64(total-verified))
		emit("gremlin_recovery_skipped_total", "Reports explicitly skipping health verification", "gauge", float64(skipped))
		emit("gremlin_recovery_unknown_total", "Legacy reports with ambiguous or missing recovery evidence", "gauge", float64(unknown))
		emit("gremlin_recovery_coverage_percent", "Percentage of reports with verified recovery measurements", "gauge", coverage)
		emit("gremlin_last_recovery_status", "Latest recovery: 0 unverified, 1 verified, 2 skipped, 3 unknown legacy or no reports", "gauge", lastStatus)
		names := make([]string, 0, len(byType))
		for name := range byType {
			names = append(names, name)
		}
		sort.Strings(names)
		labeled := func(name, help, kind string, value func(*typeStats) float64) {
			fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
			escape := strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\"", "\\\"")
			for _, attack := range names {
				fmt.Fprintf(w, "%s{attack=\"%s\"} %g\n", name, escape.Replace(attack), value(byType[attack]))
			}
		}
		labeled("gremlin_attacks_by_type_total", "Saved attacks by type", "counter", func(st *typeStats) float64 { return float64(st.total) })
		labeled("gremlin_attacks_failed_by_type_total", "Unsuccessful report flags by type", "counter", func(st *typeStats) float64 { return float64(st.failed) })
		labeled("gremlin_recovery_verified_by_type", "Verified recovery measurements by type", "gauge", func(st *typeStats) float64 { return float64(st.verified) })
		labeled("gremlin_avg_recovery_ms_by_type", "Mean verified recovery time; NaN if no verified measurements", "gauge", func(st *typeStats) float64 {
			if st.verified == 0 {
				return math.NaN()
			}
			return float64(st.recovery) / float64(st.verified)
		})
		labeled("gremlin_avg_duration_ms_by_type", "Mean recorded attack duration by type", "gauge", func(st *typeStats) float64 { return float64(st.duration) / float64(st.total) })
	})
}

func (c *Collector) Serve(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", c.Handler())
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return server.ListenAndServe()
}
