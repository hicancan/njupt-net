package guard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	startDetachedProcess = realStartDetachedProcess
	processExists        = defaultProcessExists
	killPID              = defaultKillPID
	findLegacyPIDs       = defaultFindLegacyPIDs
	gracefulStopTimeout  = 5 * time.Second
	gracefulStopPoll     = 200 * time.Millisecond
)

// Supervisor manages background guard processes.
type Supervisor struct {
	store      *StateStore
	executable string
	workDir    string
}

// NewSupervisor creates the guard process supervisor.
func NewSupervisor(store *StateStore, executable, workDir string) *Supervisor {
	return &Supervisor{
		store:      store,
		executable: executable,
		workDir:    workDir,
	}
}

// Status returns the current process status from pid files.
func (s *Supervisor) Status(ctx context.Context) (*ControlResult, error) {
	_ = ctx
	pid, _ := s.store.ReadPID(s.store.WorkerPIDFile())
	if pid > 0 && processExists(pid) {
		return &ControlResult{
			Running:  true,
			PID:      pid,
			LogPath:  s.store.CurrentLogPath(),
			StateDir: s.store.StateDir(),
		}, nil
	}
	return &ControlResult{
		Running:  false,
		LogPath:  s.store.CurrentLogPath(),
		StateDir: s.store.StateDir(),
	}, nil
}

// Stop stops the current Go guard, if any.
func (s *Supervisor) Stop(ctx context.Context) (*ControlResult, error) {
	pids := s.readKnownPIDs()
	if len(pids) == 0 {
		s.store.ClearStopRequest()
		return &ControlResult{
			Running:  false,
			LogPath:  s.store.CurrentLogPath(),
			StateDir: s.store.StateDir(),
			PID:      0,
		}, nil
	}

	if err := s.store.WriteStopRequest("guard stop requested"); err != nil {
		return nil, err
	}
	if s.waitForExit(ctx, pids) {
		s.cleanupStoppedState()
		return &ControlResult{
			Running:  false,
			LogPath:  s.store.CurrentLogPath(),
			StateDir: s.store.StateDir(),
			PID:      0,
		}, nil
	}

	for _, pid := range pids {
		if processExists(pid) {
			if err := killPID(pid); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return nil, err
			}
		}
	}
	s.cleanupStoppedState()
	return &ControlResult{
		Running:  false,
		LogPath:  s.store.CurrentLogPath(),
		StateDir: s.store.StateDir(),
		PID:      0,
	}, nil
}

// Start launches a detached `guard run` child process.
func (s *Supervisor) Start(ctx context.Context, args []string, replace bool) (*ControlResult, error) {
	_ = ctx
	current, err := s.Status(ctx)
	if err == nil && current.Running && !replace {
		return current, nil
	}
	if replace {
		if _, err := s.Stop(ctx); err != nil {
			return nil, err
		}
	}
	logPath, err := s.store.NextLogPath()
	if err != nil {
		return nil, err
	}
	startArgs := append([]string{}, args...)
	startArgs = append(startArgs, "--log-file", logPath)
	pid, err := startDetachedProcess(s.executable, startArgs, s.workDir, logPath)
	if err != nil {
		return nil, err
	}
	if err := s.store.WritePID(s.store.LauncherPIDFile(), pid); err != nil {
		return nil, err
	}
	return &ControlResult{
		Running:  true,
		PID:      pid,
		LogPath:  logPath,
		StateDir: s.store.StateDir(),
	}, nil
}

// StopLegacy stops the historical Python/Powershell guard when present.
func (s *Supervisor) StopLegacy() (bool, error) {
	legacyDir := s.store.LegacyStateDir()
	killed := false
	seen := map[int]struct{}{}
	for _, name := range []string{"terminal.pid", "worker.pid"} {
		pidPath := filepath.Join(legacyDir, name)
		payload, err := os.ReadFile(pidPath)
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(payload)))
		if err != nil || pid <= 0 {
			_ = os.Remove(pidPath)
			continue
		}
		if processExists(pid) {
			if err := killPID(pid); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return killed, err
			}
			killed = true
			seen[pid] = struct{}{}
		}
		_ = os.Remove(pidPath)
	}
	pids, err := findLegacyPIDs()
	if err != nil {
		return killed, err
	}
	for _, pid := range pids {
		if _, ok := seen[pid]; ok {
			continue
		}
		if processExists(pid) {
			if err := killPID(pid); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return killed, err
			}
			killed = true
		}
	}
	return killed, nil
}

func realStartDetachedProcess(executable string, args []string, workDir, logPath string) (int, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return 0, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer logFile.Close()
	nullFile, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return 0, err
	}
	defer nullFile.Close()

	cmd := exec.Command(executable, args...)
	cmd.Dir = workDir
	cmd.Stdout = nullFile
	cmd.Stderr = nullFile
	cmd.Stdin = nil
	cmd.SysProcAttr = detachedSysProcAttr()
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

func defaultKillPID(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

// BuildRunArgs assembles the detached `guard run` command line.
func BuildRunArgs(rootArgs []string, runArgs []string) []string {
	result := make([]string, 0, len(rootArgs)+1+len(runArgs))
	result = append(result, rootArgs...)
	result = append(result, "guard", "run")
	result = append(result, runArgs...)
	return result
}

// OpenForegroundWriter returns a writer that mirrors to stdout and the log file.
func OpenForegroundWriter(logPath string, stdout io.Writer) (io.Writer, func(), error) {
	if strings.TrimSpace(logPath) == "" {
		if stdout == nil {
			stdout = io.Discard
		}
		return stdout, func() {}, nil
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open guard log: %w", err)
	}
	if stdout == nil {
		stdout = io.Discard
	}
	return io.MultiWriter(stdout, file), func() { _ = file.Close() }, nil
}

func (s *Supervisor) readKnownPIDs() []int {
	paths := []string{s.store.WorkerPIDFile(), s.store.LauncherPIDFile()}
	seen := map[int]struct{}{}
	pids := []int{}
	for _, pidPath := range paths {
		pid, err := s.store.ReadPID(pidPath)
		if err != nil || pid <= 0 {
			s.store.RemovePID(pidPath)
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		pids = append(pids, pid)
	}
	return pids
}

func (s *Supervisor) waitForExit(ctx context.Context, pids []int) bool {
	deadline := time.NewTimer(gracefulStopTimeout)
	ticker := time.NewTicker(gracefulStopPoll)
	defer deadline.Stop()
	defer ticker.Stop()

	for {
		if !anyProcessExists(pids) {
			return true
		}
		select {
		case <-ctx.Done():
			return !anyProcessExists(pids)
		case <-deadline.C:
			return !anyProcessExists(pids)
		case <-ticker.C:
		}
	}
}

func (s *Supervisor) cleanupStoppedState() {
	s.store.RemovePID(s.store.WorkerPIDFile())
	s.store.RemovePID(s.store.LauncherPIDFile())
	s.store.ClearStopRequest()
}

func anyProcessExists(pids []int) bool {
	for _, pid := range pids {
		if processExists(pid) {
			return true
		}
	}
	return false
}
