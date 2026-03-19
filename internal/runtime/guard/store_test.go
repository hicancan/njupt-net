package guard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateStorePruneLogsKeepsNewestPairs(t *testing.T) {
	store, err := NewStateStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	logs := []string{
		filepath.Join(store.LogsDir(), "guard-20260101-000000.log"),
		filepath.Join(store.LogsDir(), "guard-20260101-000001.log"),
		filepath.Join(store.LogsDir(), "guard-20260101-000002.log"),
	}
	for _, logPath := range logs {
		if err := os.WriteFile(logPath, []byte("log"), 0o644); err != nil {
			t.Fatalf("write log: %v", err)
		}
		if err := os.WriteFile(store.EventPathForLog(logPath), []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("write event log: %v", err)
		}
	}

	if err := store.PruneLogs(2); err != nil {
		t.Fatalf("prune logs: %v", err)
	}
	if _, err := os.Stat(logs[0]); !os.IsNotExist(err) {
		t.Fatalf("expected oldest log removed, got %v", err)
	}
	if _, err := os.Stat(store.EventPathForLog(logs[0])); !os.IsNotExist(err) {
		t.Fatalf("expected oldest event log removed, got %v", err)
	}
	for _, logPath := range logs[1:] {
		if _, err := os.Stat(logPath); err != nil {
			t.Fatalf("expected retained log %s: %v", logPath, err)
		}
		if _, err := os.Stat(store.EventPathForLog(logPath)); err != nil {
			t.Fatalf("expected retained event log %s: %v", store.EventPathForLog(logPath), err)
		}
	}
}
