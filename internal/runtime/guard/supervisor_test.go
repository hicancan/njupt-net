package guard

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSupervisorStartStopStatus(t *testing.T) {
	store, err := NewStateStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	oldStart := startDetachedProcess
	oldExists := processExists
	oldKill := killPID
	oldFindLegacy := findLegacyPIDs
	defer func() {
		startDetachedProcess = oldStart
		processExists = oldExists
		killPID = oldKill
		findLegacyPIDs = oldFindLegacy
	}()

	startCalled := false
	startDetachedProcess = func(executable string, args []string, workDir, logPath string) (int, error) {
		startCalled = true
		if executable != "njupt-net.exe" {
			t.Fatalf("unexpected executable: %s", executable)
		}
		if logPath == "" {
			t.Fatal("expected log path")
		}
		foundLogFlag := false
		for index := 0; index < len(args)-1; index++ {
			if args[index] == "--log-file" && args[index+1] == logPath {
				foundLogFlag = true
				break
			}
		}
		if !foundLogFlag {
			t.Fatalf("expected --log-file %q in args: %#v", logPath, args)
		}
		return 4321, nil
	}
	processExists = func(pid int) bool {
		return pid == 4321
	}
	killed := []int{}
	killPID = func(pid int) error {
		killed = append(killed, pid)
		return nil
	}

	supervisor := NewSupervisor(store, "njupt-net.exe", t.TempDir())
	startResult, err := supervisor.Start(context.Background(), []string{"guard", "run", "--yes"}, false)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if !startCalled || !startResult.Running || startResult.PID != 4321 {
		t.Fatalf("unexpected start result: %#v", startResult)
	}
	if _, err := os.Stat(store.LauncherPIDFile()); err != nil {
		t.Fatalf("expected launcher pid file: %v", err)
	}

	if err := store.WritePID(store.WorkerPIDFile(), 4321); err != nil {
		t.Fatalf("write worker pid: %v", err)
	}
	status, err := supervisor.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !status.Running || status.PID != 4321 {
		t.Fatalf("unexpected status: %#v", status)
	}

	stopResult, err := supervisor.Stop(context.Background())
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if stopResult.Running {
		t.Fatalf("expected stopped result: %#v", stopResult)
	}
	if len(killed) == 0 {
		t.Fatal("expected process kill")
	}
	if _, err := os.Stat(store.WorkerPIDFile()); !os.IsNotExist(err) {
		t.Fatalf("expected worker pid removed, got %v", err)
	}
}

func TestSupervisorStopLegacy(t *testing.T) {
	stateDir := t.TempDir()
	store, err := NewStateStore(filepath.Join(stateDir, "guard"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	legacyDir := filepath.Join(stateDir, "w-guard")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "terminal.pid"), []byte("88"), 0o644); err != nil {
		t.Fatalf("write terminal pid: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "worker.pid"), []byte("99"), 0o644); err != nil {
		t.Fatalf("write worker pid: %v", err)
	}

	oldExists := processExists
	oldKill := killPID
	oldFindLegacy := findLegacyPIDs
	defer func() {
		processExists = oldExists
		killPID = oldKill
		findLegacyPIDs = oldFindLegacy
	}()

	killed := []int{}
	processExists = func(pid int) bool { return true }
	killPID = func(pid int) error {
		killed = append(killed, pid)
		return nil
	}
	findLegacyPIDs = func() ([]int, error) { return nil, nil }

	supervisor := NewSupervisor(store, "", "")
	legacyKilled, err := supervisor.StopLegacy()
	if err != nil {
		t.Fatalf("stop legacy: %v", err)
	}
	if !legacyKilled || len(killed) != 2 {
		t.Fatalf("unexpected legacy stop result: killed=%v pids=%v", legacyKilled, killed)
	}
}
