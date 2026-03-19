package guard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type stopRequest struct {
	Reason    string `json:"reason,omitempty"`
	Requested string `json:"requested,omitempty"`
}

// StateStore manages guard state, log pointers, and pid files.
type StateStore struct {
	stateDir string
	now      func() time.Time
}

// NewStateStore resolves and creates the guard state directory.
func NewStateStore(stateDir string) (*StateStore, error) {
	resolved, err := filepath.Abs(stateDir)
	if err != nil {
		return nil, err
	}
	store := &StateStore{
		stateDir: resolved,
		now:      time.Now,
	}
	if err := store.Ensure(); err != nil {
		return nil, err
	}
	return store, nil
}

// Ensure creates the state and logs directories.
func (s *StateStore) Ensure() error {
	if err := os.MkdirAll(s.stateDir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(s.LogsDir(), 0o755)
}

// StateDir returns the resolved state directory.
func (s *StateStore) StateDir() string { return s.stateDir }

// LogsDir returns the logs directory.
func (s *StateStore) LogsDir() string { return filepath.Join(s.stateDir, "logs") }

// StatusFile returns the persisted status path.
func (s *StateStore) StatusFile() string { return filepath.Join(s.stateDir, "status.json") }

// CurrentLogFile returns the pointer file for the current log.
func (s *StateStore) CurrentLogFile() string { return filepath.Join(s.stateDir, "current-log.txt") }

// CurrentEventFile returns the pointer file for the current structured event log.
func (s *StateStore) CurrentEventFile() string {
	return filepath.Join(s.stateDir, "current-events.txt")
}

// WorkerPIDFile returns the worker pid file path.
func (s *StateStore) WorkerPIDFile() string { return filepath.Join(s.stateDir, "worker.pid") }

// LauncherPIDFile returns the launcher pid file path.
func (s *StateStore) LauncherPIDFile() string { return filepath.Join(s.stateDir, "launcher.pid") }

// StopRequestFile returns the graceful stop request path.
func (s *StateStore) StopRequestFile() string { return filepath.Join(s.stateDir, "stop-request.json") }

// LegacyStateDir returns the default legacy Python guard state directory.
func (s *StateStore) LegacyStateDir() string {
	return filepath.Join(filepath.Dir(s.stateDir), "w-guard")
}

// NextLogPath returns a fresh log file path and updates current-log.txt.
func (s *StateStore) NextLogPath() (string, error) {
	logPath := filepath.Join(s.LogsDir(), "guard-"+s.now().Format("20060102-150405")+".log")
	if err := s.UseLogPath(logPath); err != nil {
		return "", err
	}
	_ = s.PruneLogs(10)
	return logPath, nil
}

// UseLogPath updates current-log.txt to point at the provided path.
func (s *StateStore) UseLogPath(logPath string) error {
	if err := os.WriteFile(s.CurrentLogFile(), []byte(logPath), 0o644); err != nil {
		return err
	}
	return os.WriteFile(s.CurrentEventFile(), []byte(s.EventPathForLog(logPath)), 0o644)
}

// CurrentLogPath reads the current log pointer.
func (s *StateStore) CurrentLogPath() string {
	payload, err := os.ReadFile(s.CurrentLogFile())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(payload))
}

// CurrentEventPath reads the current structured event log pointer.
func (s *StateStore) CurrentEventPath() string {
	payload, err := os.ReadFile(s.CurrentEventFile())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(payload))
}

// EventPathForLog returns the JSONL event log paired with the text log.
func (s *StateStore) EventPathForLog(logPath string) string {
	trimmed := strings.TrimSpace(logPath)
	if trimmed == "" {
		return ""
	}
	if strings.HasSuffix(trimmed, ".log") {
		return strings.TrimSuffix(trimmed, ".log") + ".events.jsonl"
	}
	return trimmed + ".events.jsonl"
}

// WriteStatus persists the current guard status.
func (s *StateStore) WriteStatus(status Status) error {
	payload, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.StatusFile(), payload, 0o644)
}

// ReadStatus reads the persisted guard status.
func (s *StateStore) ReadStatus() (*Status, error) {
	payload, err := os.ReadFile(s.StatusFile())
	if err != nil {
		return nil, err
	}
	var status Status
	if err := json.Unmarshal(payload, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// WritePID persists a process identifier to the specified pid file.
func (s *StateStore) WritePID(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
}

// ReadPID reads one pid file.
func (s *StateStore) ReadPID(path string) (int, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	value := strings.TrimSpace(string(payload))
	if value == "" {
		return 0, fmt.Errorf("empty pid file")
	}
	return strconv.Atoi(value)
}

// RemovePID removes the specified pid file if it exists.
func (s *StateStore) RemovePID(path string) {
	_ = os.Remove(path)
}

// WriteStopRequest records a graceful shutdown request.
func (s *StateStore) WriteStopRequest(reason string) error {
	payload, err := json.Marshal(stopRequest{
		Reason:    strings.TrimSpace(reason),
		Requested: s.now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	return os.WriteFile(s.StopRequestFile(), payload, 0o644)
}

// ReadStopRequest returns the current graceful shutdown request, if any.
func (s *StateStore) ReadStopRequest() (string, bool) {
	payload, err := os.ReadFile(s.StopRequestFile())
	if err != nil {
		return "", false
	}
	var request stopRequest
	if err := json.Unmarshal(payload, &request); err == nil {
		return strings.TrimSpace(request.Reason), true
	}
	return "", true
}

// StopRequested reports whether a graceful shutdown request is present.
func (s *StateStore) StopRequested() bool {
	_, ok := s.ReadStopRequest()
	return ok
}

// ClearStopRequest removes the graceful shutdown marker.
func (s *StateStore) ClearStopRequest() {
	_ = os.Remove(s.StopRequestFile())
}

// PruneLogs keeps only the newest paired text/event log sets.
func (s *StateStore) PruneLogs(maxFiles int) error {
	if maxFiles <= 0 {
		return nil
	}
	entries, err := filepath.Glob(filepath.Join(s.LogsDir(), "guard-*.log"))
	if err != nil {
		return err
	}
	if len(entries) <= maxFiles {
		return nil
	}
	sort.Strings(entries)
	for _, stale := range entries[:len(entries)-maxFiles] {
		_ = os.Remove(stale)
		_ = os.Remove(s.EventPathForLog(stale))
	}
	return nil
}
