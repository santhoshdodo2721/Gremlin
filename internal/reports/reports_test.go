package reports

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reports.json")
	store := NewStore(path)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := store.Save(Report{Attack: "test"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	list, err := store.List()
	if err != nil || len(list) != 20 {
		t.Fatalf("count=%d error=%v", len(list), err)
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".gremlin-reports-*"))
	if len(files) != 0 {
		t.Fatal("temporary files leaked")
	}
}
func TestCorruptStorePreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reports.json")
	os.WriteFile(path, []byte("broken"), 0600)
	if err := NewStore(path).Save(Report{}); err == nil {
		t.Fatal("expected error")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "broken" {
		t.Fatal("overwrote corrupt store")
	}
}
