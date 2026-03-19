package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	runtimeguard "github.com/hicancan/njupt-net-cli/internal/runtime/guard"
)

func TestRootHelpDoesNotRequireConfig(t *testing.T) {
	cmd := newRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--config", filepath.Join(t.TempDir(), "missing.json"), "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("help should not require config: %v", err)
	}
	if !strings.Contains(stdout.String(), "Kernel-first terminal system for NJUPT Self and Portal workflows.") {
		t.Fatalf("unexpected help output: %s", stdout.String())
	}
}

func TestGuardStatusWithExplicitStateDirDoesNotRequireConfig(t *testing.T) {
	store, err := runtimeguard.NewStateStore(t.TempDir())
	if err != nil {
		t.Fatalf("new state store: %v", err)
	}
	if err := store.UseLogPath(filepath.Join(store.LogsDir(), "guard-test.log")); err != nil {
		t.Fatalf("use log path: %v", err)
	}
	if err := store.WriteStatus(runtimeguard.Status{
		Running:        true,
		Health:         runtimeguard.HealthHealthy,
		DesiredProfile: "B",
		ScheduleWindow: "day",
		Binding: runtimeguard.BindingStatus{
			OK: true,
		},
		Connectivity: runtimeguard.ConnectivityStatus{
			FinalOK: true,
		},
		Portal: runtimeguard.PortalStatus{
			OK: true,
		},
		Cycle: runtimeguard.CycleStatus{
			RecoveryStep: "healthy",
		},
		Log: runtimeguard.LogStatus{
			Path: store.CurrentLogPath(),
		},
	}); err != nil {
		t.Fatalf("write status: %v", err)
	}

	cmd := newRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"--config", filepath.Join(store.StateDir(), "missing.json"),
		"--output", "json",
		"guard", "status",
		"--state-dir", store.StateDir(),
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("guard status should not require config when state-dir is explicit: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode json output: %v", err)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in payload: %#v", payload)
	}
	if got := data["desiredProfile"]; got != "B" {
		t.Fatalf("expected desiredProfile B, got %#v", got)
	}
	if got := data["health"]; got != string(runtimeguard.HealthStopped) {
		t.Fatalf("expected health=%s, got %#v", runtimeguard.HealthStopped, got)
	}
}

func TestGuardStopWithExplicitStateDirDoesNotRequireConfig(t *testing.T) {
	store, err := runtimeguard.NewStateStore(t.TempDir())
	if err != nil {
		t.Fatalf("new state store: %v", err)
	}

	cmd := newRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"--config", filepath.Join(store.StateDir(), "missing.json"),
		"--yes",
		"guard", "stop",
		"--state-dir", store.StateDir(),
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("guard stop should not require config when state-dir is explicit: %v", err)
	}
	if !strings.Contains(stdout.String(), "guard stopped") {
		t.Fatalf("unexpected stop output: %s", stdout.String())
	}
}
