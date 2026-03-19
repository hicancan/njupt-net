package guard

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hicancan/njupt-net-cli/internal/config"
	"github.com/hicancan/njupt-net-cli/internal/workflow"
)

type fakeRunnerProber struct {
	connectivityOK  bool
	connectivityMsg string
	selection       workflow.LocalIPSelection
}

func (p fakeRunnerProber) CheckConnectivity(ctx context.Context) (bool, string) {
	_ = ctx
	return p.connectivityOK, p.connectivityMsg
}

func (p fakeRunnerProber) DetectLocalIPv4(ctx context.Context) (workflow.LocalIPSelection, error) {
	_ = ctx
	return p.selection, nil
}

func TestRunnerExecuteCycleEmitsStructuredEvents(t *testing.T) {
	store, err := NewStateStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	logPath := filepath.Join(store.LogsDir(), "guard-test.log")
	if err := store.UseLogPath(logPath); err != nil {
		t.Fatalf("use log path: %v", err)
	}

	runner, err := NewRunner(testGuardSettings(), store, io.Discard)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	defer runner.close()
	runner.prober = fakeRunnerProber{
		connectivityOK:  false,
		connectivityMsg: "offline",
		selection: workflow.LocalIPSelection{
			SelectedIP:      "10.163.177.138",
			RoutedIP:        "10.163.177.138",
			SelectionReason: "matches-routed-ip,campus-ip,preferred-interface",
		},
	}
	runner.now = func() time.Time {
		return time.Date(2026, 3, 19, 23, 45, 0, 0, time.FixedZone("CST", 8*3600))
	}
	runner.guardCycle = func(ctx context.Context, env workflow.GuardEnvironment, input workflow.GuardCycleInput) (*workflow.GuardCycleResult, error) {
		_ = ctx
		_ = env
		return &workflow.GuardCycleResult{
			DesiredProfile:     input.DesiredProfile,
			ScheduleWindow:     input.ScheduleWindow,
			ForceSwitch:        input.ForceSwitch,
			ForceBindingCheck:  input.ForceBindingCheck,
			BindingOK:          false,
			BindingMessage:     "binding repair needed",
			InternetOK:         false,
			InternetMessage:    "offline after retry",
			PortalLoginOK:      false,
			PortalLoginMessage: "portal retry failed",
			RecoveryStep:       "binding-repair-then-portal-login",
			BindingRepair: &workflow.BindingRepairResult{
				TargetProfile: input.DesiredProfile,
				Action:        "target-bind-failed",
			},
		}, errors.New("still offline")
	}

	status, err := runner.executeCycle(context.Background())
	if err == nil {
		t.Fatal("expected degraded cycle error")
	}
	if status == nil {
		t.Fatal("expected status")
	}
	if status.Health != HealthDegraded {
		t.Fatalf("expected degraded health, got %#v", status.Health)
	}
	if status.Binding.Audited != true || status.Cycle.SwitchTriggered != true || status.Cycle.SwitchCompleted {
		t.Fatalf("unexpected typed cycle status: %#v", status)
	}
	if status.Connectivity.Probe == nil || status.Connectivity.Probe.SelectedIP != "10.163.177.138" {
		t.Fatalf("expected persisted probe selection, got %#v", status.Connectivity.Probe)
	}

	kinds := readEventKinds(t, store.CurrentEventPath())
	for _, expected := range []EventKind{EventBindingAudit, EventPortalLogin, EventBindingRepair, EventDegraded} {
		if !containsEventKind(kinds, expected) {
			t.Fatalf("expected event kind %s in %v", expected, kinds)
		}
	}
	if containsEventKind(kinds, EventScheduleSwitch) {
		t.Fatalf("did not expect completed schedule-switch event in degraded cycle: %v", kinds)
	}
}

func TestRunnerRunEmitsStartupAndShutdown(t *testing.T) {
	store, err := NewStateStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	logPath, err := store.NextLogPath()
	if err != nil {
		t.Fatalf("next log path: %v", err)
	}
	_ = logPath

	runner, err := NewRunner(testGuardSettings(), store, io.Discard)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	runner.guardCycle = func(ctx context.Context, env workflow.GuardEnvironment, input workflow.GuardCycleInput) (*workflow.GuardCycleResult, error) {
		_ = ctx
		_ = env
		return &workflow.GuardCycleResult{
			DesiredProfile:     input.DesiredProfile,
			ScheduleWindow:     input.ScheduleWindow,
			ForceSwitch:        input.ForceSwitch,
			ForceBindingCheck:  input.ForceBindingCheck,
			BindingOK:          true,
			BindingMessage:     "binding audit ok",
			InternetOK:         true,
			InternetMessage:    "internet ok",
			PortalLoginOK:      true,
			PortalLoginMessage: "portal login not needed",
			RecoveryStep:       "healthy",
		}, nil
	}
	runner.prober = fakeRunnerProber{
		connectivityOK:  true,
		connectivityMsg: "internet ok",
		selection: workflow.LocalIPSelection{
			SelectedIP:      "10.163.177.138",
			RoutedIP:        "10.163.177.138",
			SelectionReason: "matches-routed-ip,campus-ip,preferred-interface",
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = store.WriteStopRequest("test stop")
	}()

	if err := runner.Run(ctx, false); err != nil {
		t.Fatalf("run: %v", err)
	}

	kinds := readEventKinds(t, store.CurrentEventPath())
	for _, expected := range []EventKind{EventStartup, EventShutdown} {
		if !containsEventKind(kinds, expected) {
			t.Fatalf("expected event kind %s in %v", expected, kinds)
		}
	}
}

func TestRunnerHealthyCycleDoesNotEmitDegradedWhenPortalWasNotNeeded(t *testing.T) {
	store, err := NewStateStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	logPath, err := store.NextLogPath()
	if err != nil {
		t.Fatalf("next log path: %v", err)
	}
	_ = logPath

	runner, err := NewRunner(testGuardSettings(), store, io.Discard)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	defer runner.close()
	runner.prober = fakeRunnerProber{
		connectivityOK:  true,
		connectivityMsg: "internet ok",
		selection: workflow.LocalIPSelection{
			SelectedIP:      "10.163.177.138",
			RoutedIP:        "10.163.177.138",
			SelectionReason: "matches-routed-ip,campus-ip,preferred-interface",
		},
	}
	runner.guardCycle = func(ctx context.Context, env workflow.GuardEnvironment, input workflow.GuardCycleInput) (*workflow.GuardCycleResult, error) {
		_ = ctx
		_ = env
		return &workflow.GuardCycleResult{
			DesiredProfile:     input.DesiredProfile,
			ScheduleWindow:     input.ScheduleWindow,
			ForceSwitch:        input.ForceSwitch,
			ForceBindingCheck:  input.ForceBindingCheck,
			BindingOK:          true,
			BindingMessage:     "binding audit ok",
			InternetOK:         true,
			InternetMessage:    "internet ok",
			PortalLoginOK:      false,
			PortalLoginMessage: "portal retry failed after internet recovered",
			RecoveryStep:       "portal-login",
		}, errors.New("portal retry failed")
	}

	status, err := runner.executeCycle(context.Background())
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if status.Health != HealthHealthy {
		t.Fatalf("expected healthy status when internet recovered, got %#v", status.Health)
	}
	kinds := readEventKinds(t, store.CurrentEventPath())
	if containsEventKind(kinds, EventDegraded) {
		t.Fatalf("did not expect degraded event when internet recovered: %v", kinds)
	}
}

func testGuardSettings() Settings {
	location, _ := time.LoadLocation("Asia/Shanghai")
	return Settings{
		ProbeInterval:        10 * time.Millisecond,
		BindingCheckInterval: 10 * time.Millisecond,
		Location:             location,
		Accounts: map[string]config.AccountConfig{
			"B": {Username: "b-user", Password: "b-pass"},
			"W": {Username: "w-user", Password: "w-pass"},
		},
		Broadband: config.BroadbandConfig{
			Account:  "cmcc-user",
			Password: "cmcc-pass",
		},
		PortalISP: "mobile",
		Schedule: ScheduleConfig{
			DayProfile:   "B",
			NightProfile: "W",
			NightStart:   "23:30",
			NightEnd:     "07:00",
		},
	}
}

func readEventKinds(t *testing.T, path string) []EventKind {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	lines := bytesToLines(payload)
	kinds := make([]EventKind, 0, len(lines))
	for _, line := range lines {
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("unmarshal event %q: %v", line, err)
		}
		kinds = append(kinds, event.Kind)
	}
	return kinds
}

func bytesToLines(payload []byte) []string {
	lines := []string{}
	start := 0
	for index, b := range payload {
		if b != '\n' {
			continue
		}
		if index > start {
			lines = append(lines, string(payload[start:index]))
		}
		start = index + 1
	}
	if start < len(payload) {
		lines = append(lines, string(payload[start:]))
	}
	return lines
}

func containsEventKind(kinds []EventKind, target EventKind) bool {
	for _, kind := range kinds {
		if kind == target {
			return true
		}
	}
	return false
}
