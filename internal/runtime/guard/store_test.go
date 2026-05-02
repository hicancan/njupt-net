package guard

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

	if err := store.PruneLogs(LogRetentionPolicy{MaxFiles: 2}); err != nil {
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

func TestStateStorePruneLogsRemovesFilesOlderThanRetention(t *testing.T) {
	store, err := NewStateStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	store.now = func() time.Time {
		return time.Date(2026, time.January, 10, 12, 0, 0, 0, time.Local)
	}

	oldLog := filepath.Join(store.LogsDir(), "guard-20260103.log")
	boundaryLog := filepath.Join(store.LogsDir(), "guard-20260104.log")
	currentLog := filepath.Join(store.LogsDir(), "guard-20260110.log")
	for _, logPath := range []string{oldLog, boundaryLog, currentLog} {
		if err := os.WriteFile(logPath, []byte("log"), 0o644); err != nil {
			t.Fatalf("write log: %v", err)
		}
		if err := os.WriteFile(store.EventPathForLog(logPath), []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("write event log: %v", err)
		}
	}

	if err := store.PruneLogs(LogRetentionPolicy{RetentionDays: 7, MaxFiles: 14}); err != nil {
		t.Fatalf("prune logs: %v", err)
	}
	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Fatalf("expected old log removed, got %v", err)
	}
	if _, err := os.Stat(store.EventPathForLog(oldLog)); !os.IsNotExist(err) {
		t.Fatalf("expected old event log removed, got %v", err)
	}
	for _, logPath := range []string{boundaryLog, currentLog} {
		if _, err := os.Stat(logPath); err != nil {
			t.Fatalf("expected retained log %s: %v", logPath, err)
		}
	}
}

func TestStateStoreNextLogPathUsesDailyFile(t *testing.T) {
	store, err := NewStateStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	store.now = func() time.Time {
		return time.Date(2026, time.January, 10, 12, 0, 0, 0, time.Local)
	}
	first, err := store.NextLogPath(LogRetentionPolicy{RetentionDays: 7, MaxFiles: 14})
	if err != nil {
		t.Fatalf("next log path: %v", err)
	}
	store.now = func() time.Time {
		return time.Date(2026, time.January, 10, 23, 0, 0, 0, time.Local)
	}
	second, err := store.NextLogPath(LogRetentionPolicy{RetentionDays: 7, MaxFiles: 14})
	if err != nil {
		t.Fatalf("next log path: %v", err)
	}
	if first != second {
		t.Fatalf("expected same daily log path, first=%q second=%q", first, second)
	}
	if got, want := filepath.Base(first), "guard-20260110.log"; got != want {
		t.Fatalf("unexpected daily log name: got %q want %q", got, want)
	}
}
