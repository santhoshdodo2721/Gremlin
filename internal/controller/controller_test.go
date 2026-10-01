package controller

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"gremlin-in-a-box/internal/plugins"
	"gremlin-in-a-box/internal/reports"
)

type testPlugin struct{ success bool }

func (p testPlugin) Name() string     { return "controller-test" }
func (p testPlugin) Describe() string { return "test" }
func (p testPlugin) Run(context.Context, string, plugins.Params) (plugins.Result, error) {
	return plugins.Result{Success: p.success, Message: "injected"}, nil
}
func TestOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		success   bool
		url       string
		wantError bool
	}{
		{"successful attack", true, "", false},
		{"plugin failure", false, "", true},
		{"invalid recovery URL", true, "://bad", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plugins.Register(testPlugin{tc.success})
			store := reports.NewStore(filepath.Join(t.TempDir(), "reports.json"))
			c := New(slog.New(slog.NewTextHandler(io.Discard, nil)), store, tc.url)
			err := c.RunAttack(context.Background(), "controller-test", "test-target", nil)
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v", err)
			}
			list, err := store.List()
			if err != nil || len(list) != 1 {
				t.Fatalf("report missing: %v", err)
			}
			if list[0].Success == tc.wantError {
				t.Fatalf("incorrect success: %+v", list[0])
			}
		})
	}
}
func TestRecoveryCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	c := &Controller{}
	start := time.Now()
	if got := c.measureRecovery(ctx, server.URL); got != -1 {
		t.Fatalf("got %d", got)
	}
	if time.Since(start) > time.Second {
		t.Fatal("recovery did not respect cancellation")
	}
}
func TestReportWriteFailure(t *testing.T) {
	plugins.Register(testPlugin{true})
	store := reports.NewStore(filepath.Join(t.TempDir(), "missing", "reports.json"))
	c := New(slog.New(slog.NewTextHandler(io.Discard, nil)), store, "")
	if err := c.RunAttack(context.Background(), "controller-test", "target", nil); err == nil {
		t.Fatal("expected report write error")
	}
}
