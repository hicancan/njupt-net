package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	runtimeguard "github.com/hicancan/njupt-net-cli/internal/runtime/guard"
	"github.com/spf13/cobra"
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

func TestCommandSurfaceMatchesREADME(t *testing.T) {
	expectedLeafCommands := []string{
		"self login",
		"self logout",
		"self status",
		"self doctor",
		"dashboard online-list",
		"dashboard login-history",
		"dashboard refresh-account-raw",
		"dashboard offline",
		"dashboard mauth get",
		"dashboard mauth toggle",
		"service binding get",
		"service binding set",
		"service consume get",
		"service consume set",
		"service mac list",
		"service migrate",
		"setting person get",
		"setting person update",
		"bill month-pay",
		"bill online-log",
		"bill operator-log",
		"portal login",
		"portal logout",
		"portal login-801",
		"portal logout-801",
		"raw get",
		"raw post",
		"guard run",
		"guard start",
		"guard stop",
		"guard status",
		"guard once",
	}

	cmd := newRootCmd()
	actualLeafCommands := collectLeafCommands("", cmd)
	slices.Sort(actualLeafCommands)
	slices.Sort(expectedLeafCommands)
	if !slices.Equal(actualLeafCommands, expectedLeafCommands) {
		t.Fatalf("unexpected leaf command surface:\n got %#v\nwant %#v", actualLeafCommands, expectedLeafCommands)
	}

	for _, docPath := range []string{filepath.Join("..", "..", "README.md"), filepath.Join("..", "..", "README.en.md")} {
		payload, err := os.ReadFile(docPath)
		if err != nil {
			t.Fatalf("read %s: %v", docPath, err)
		}
		content := string(payload)
		for _, command := range expectedLeafCommands {
			parts := strings.Fields(command)
			leaf := parts[len(parts)-1]
			if !strings.Contains(content, leaf) {
				t.Fatalf("expected %s to mention command leaf %q", docPath, leaf)
			}
		}
		for _, topLevel := range []string{"self", "dashboard", "service", "setting", "bill", "portal", "raw", "guard"} {
			if !strings.Contains(content, "`"+topLevel+"`") && !strings.Contains(content, "- `"+topLevel+"`") {
				t.Fatalf("expected %s to mention top-level command %q", docPath, topLevel)
			}
		}
	}
}

func collectLeafCommands(prefix string, cmd *cobra.Command) []string {
	if cmd == nil {
		return nil
	}
	children := cmd.Commands()
	if len(children) == 0 {
		command := strings.TrimSpace(strings.Join([]string{prefix, cmd.Name()}, " "))
		if command == "" || command == "help" || strings.HasPrefix(command, "completion ") {
			return nil
		}
		return []string{command}
	}
	base := strings.TrimSpace(strings.Join([]string{prefix, cmd.Name()}, " "))
	if cmd.Name() == "njupt-net" {
		base = ""
	}
	out := []string{}
	for _, child := range children {
		if child.Name() == "help" || child.Name() == "completion" {
			continue
		}
		out = append(out, collectLeafCommands(base, child)...)
	}
	return out
}
