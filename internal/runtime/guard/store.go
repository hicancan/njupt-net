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

// LogRetentionPolicy limits guard text/event log growth.
type LogRetentionPolicy struct {
	RetentionDays int
	MaxFiles      int
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

// NextLogPath returns the current daily log file path and updates current-log.txt.
func (s *StateStore) NextLogPath(policy LogRetentionPolicy) (string, error) {
	logPath := filepath.Join(s.LogsDir(), "guard-"+s.now().Format("20060102")+".log")
	if err := s.UseLogPath(logPath); err != nil {
		return "", err
	}
	_ = s.PruneLogs(policy)
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

// PruneLogs removes stale paired text/event logs by age and count.
func (s *StateStore) PruneLogs(policy LogRetentionPolicy) error {
	if policy.RetentionDays <= 0 && policy.MaxFiles <= 0 {
		return nil
	}
	entries, err := filepath.Glob(filepath.Join(s.LogsDir(), "guard-*.log"))
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	sort.Strings(entries)

	stale := map[string]struct{}{}
	if policy.RetentionDays > 0 {
		cutoff := truncateToLocalDate(s.now()).AddDate(0, 0, -policy.RetentionDays+1)
		for _, entry := range entries {
			logDate, ok := parseLogDate(entry, s.now().Location())
			if ok && logDate.Before(cutoff) {
				stale[entry] = struct{}{}
			}
		}
	}

	if policy.MaxFiles > 0 {
		remaining := make([]string, 0, len(entries))
		for _, entry := range entries {
			if _, ok := stale[entry]; !ok {
				remaining = append(remaining, entry)
			}
		}
		if len(remaining) > policy.MaxFiles {
			for _, entry := range remaining[:len(remaining)-policy.MaxFiles] {
				stale[entry] = struct{}{}
			}
		}
	}

	for entry := range stale {
		_ = os.Remove(entry)
		_ = os.Remove(s.EventPathForLog(entry))
	}
	return nil
}

func truncateToLocalDate(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func parseLogDate(path string, location *time.Location) (time.Time, bool) {
	base := filepath.Base(path)
	const prefix = "guard-"
	if !strings.HasPrefix(base, prefix) || len(base) < len(prefix)+8 {
		return time.Time{}, false
	}
	raw := base[len(prefix) : len(prefix)+8]
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return time.Time{}, false
		}
	}
	if location == nil {
		location = time.Local
	}
	parsed, err := time.ParseInLocation("20060102", raw, location)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}
