package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hicancan/njupt-net-cli/internal/config"
	"github.com/hicancan/njupt-net-cli/internal/workflow"
)

func TestBuildSettingsResolvesOverridesAndFallbacks(t *testing.T) {
	cfg := &config.Config{
		Accounts: map[string]config.AccountConfig{
			"B": {Username: "b-user", Password: "b-pass"},
			"W": {Username: "w-user", Password: "w-pass"},
		},
		CMCC: config.BroadbandConfig{
			Account:  "cmcc-user",
			Password: "cmcc-pass",
		},
		Self: config.SelfConfig{
			BaseURL:        "http://10.10.244.240:8080",
			TimeoutSeconds: 10,
		},
		Portal: config.PortalConfig{
			BaseURL:          "https://10.10.244.11:802/eportal/portal",
			FallbackBaseURLs: []string{"https://p.njupt.edu.cn:802/eportal/portal"},
			ISP:              "mobile",
			TimeoutSeconds:   8,
		},
		Guard: config.GuardConfig{
			StateDir:                    filepath.Join("dist", "guard"),
			ProbeIntervalSeconds:        3,
			BindingCheckIntervalSeconds: 180,
			Timezone:                    "Asia/Shanghai",
			Schedule: config.GuardScheduleConfig{
				DayProfile:   "B",
				NightProfile: "W",
				NightStart:   "23:30",
				NightEnd:     "07:00",
			},
		},
	}

	settings, err := BuildSettings(cfg, Overrides{
		StateDir:             filepath.Join("dist", "router-guard"),
		ProbeInterval:        5,
		BindingCheckInterval: 120,
		DayProfile:           "B",
		NightProfile:         "W",
	}, true)
	if err != nil {
		t.Fatalf("build settings: %v", err)
	}
	if settings.StateDir == "" || !strings.Contains(settings.StateDir, "router-guard") {
		t.Fatalf("unexpected state dir: %#v", settings.StateDir)
	}
	if settings.ProbeInterval != 5*time.Second || settings.BindingCheckInterval != 120*time.Second {
		t.Fatalf("unexpected intervals: %#v", settings)
	}
	if settings.Schedule.DayProfile != "B" || settings.Schedule.NightProfile != "W" {
		t.Fatalf("unexpected schedule: %#v", settings.Schedule)
	}
	if settings.PortalFallbackBaseURL != "https://p.njupt.edu.cn:802/eportal/portal" {
		t.Fatalf("unexpected fallback url: %#v", settings.PortalFallbackBaseURL)
	}
	if !settings.InsecureTLS {
		t.Fatal("expected insecure tls override to propagate")
	}
}

func TestBuildSettingsRejectsInvalidInputs(t *testing.T) {
	baseConfig := &config.Config{
		Accounts: map[string]config.AccountConfig{
			"B": {Username: "b-user", Password: "b-pass"},
		},
		CMCC: config.BroadbandConfig{
			Account:  "cmcc-user",
			Password: "cmcc-pass",
		},
		Self:   config.SelfConfig{BaseURL: "http://10.10.244.240:8080", TimeoutSeconds: 10},
		Portal: config.PortalConfig{BaseURL: "https://10.10.244.11:802/eportal/portal", TimeoutSeconds: 8, ISP: "mobile"},
		Guard: config.GuardConfig{
			StateDir:                    filepath.Join("dist", "guard"),
			ProbeIntervalSeconds:        3,
			BindingCheckIntervalSeconds: 180,
			Timezone:                    "Asia/Shanghai",
			Schedule: config.GuardScheduleConfig{
				DayProfile:   "B",
				NightProfile: "B",
				NightStart:   "23:30",
				NightEnd:     "07:00",
			},
		},
	}

	if _, err := BuildSettings(baseConfig, Overrides{Timezone: "Mars/Base"}, false); err == nil {
		t.Fatal("expected invalid timezone error")
	}
	invalidIntervals := *baseConfig
	invalidIntervals.Guard.ProbeIntervalSeconds = 0
	invalidIntervals.Guard.BindingCheckIntervalSeconds = 0
	if _, err := BuildSettings(&invalidIntervals, Overrides{}, false); err == nil {
		t.Fatal("expected invalid interval error")
	}
	if _, err := BuildSettings(baseConfig, Overrides{NightProfile: "W"}, false); err == nil {
		t.Fatal("expected missing profile error")
	}
}

func TestClientFactoryHelpersAndConstructors(t *testing.T) {
	settings := testGuardSettings()
	settings.SelfBaseURL = "http://10.10.244.240:8080"
	settings.PortalBaseURL = "https://10.10.244.11:802/eportal/portal"
	settings.PortalFallbackBaseURL = "https://p.njupt.edu.cn:802/eportal/portal"
	settings.SelfTimeout = 0
	settings.PortalTimeout = 0

	factory := newClientFactory(settings)
	if _, err := factory.NewSelf(); err != nil {
		t.Fatalf("new self client: %v", err)
	}
	if _, err := factory.NewPortal(); err != nil {
		t.Fatalf("new portal client: %v", err)
	}
	accounts := toWorkflowAccounts(settings.Accounts)
	if len(accounts) != 2 || accounts["B"].Username != "b-user" {
		t.Fatalf("unexpected workflow accounts: %#v", accounts)
	}
	broadband := toWorkflowBroadband(settings.Broadband)
	if broadband.Account != "cmcc-user" {
		t.Fatalf("unexpected broadband credentials: %#v", broadband)
	}
	if got := maxDuration(0, 5*time.Second); got != 5*time.Second {
		t.Fatalf("expected fallback max duration, got %#v", got)
	}
}

func TestProbeConnectivityAndHelpers(t *testing.T) {
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer okServer.Close()

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("nope"))
	}))
	defer badServer.Close()

	probe := &Probe{
		timeout: 2 * time.Second,
		probes: []connectivityProbe{
			{
				url: badServer.URL,
				validate: func(resp *http.Response, _ []byte) bool {
					return resp.StatusCode == http.StatusNoContent
				},
			},
			{
				url: okServer.URL,
				validate: func(resp *http.Response, _ []byte) bool {
					return resp.StatusCode == http.StatusNoContent
				},
			},
		},
	}

	ok, message := probe.CheckConnectivity(context.Background())
	if !ok || !strings.Contains(message, okServer.URL) {
		t.Fatalf("expected connectivity success via ok server, got ok=%v message=%q", ok, message)
	}

	ok, message = probe.runProbe(context.Background(), connectivityProbe{
		url: "://broken",
		validate: func(resp *http.Response, _ []byte) bool {
			return resp.StatusCode == http.StatusNoContent
		},
	})
	if ok || !strings.Contains(message, "request create failed") {
		t.Fatalf("expected request creation failure, got ok=%v message=%q", ok, message)
	}

	ok, message = probe.runProbe(context.Background(), connectivityProbe{
		url: badServer.URL,
		validate: func(resp *http.Response, _ []byte) bool {
			return resp.StatusCode == http.StatusNoContent
		},
	})
	if ok || !strings.Contains(message, "unexpected response status=500") {
		t.Fatalf("expected unexpected status failure, got ok=%v message=%q", ok, message)
	}

	if !isCandidateIPv4("10.163.177.138") || isCandidateIPv4("127.0.0.1") || isCandidateIPv4("169.254.0.1") || isCandidateIPv4("192.168.137.1") {
		t.Fatal("unexpected candidate IPv4 classification")
	}
	if got := minDuration(2*time.Second, 5*time.Second); got != 2*time.Second {
		t.Fatalf("expected minimum duration, got %#v", got)
	}
	if got := typeMessage(errors.New("boom")); got == "" {
		t.Fatal("expected non-empty type message")
	}
}

func TestStateStoreReadWriteStatusAndStopRequests(t *testing.T) {
	store, err := NewStateStore(t.TempDir())
	if err != nil {
		t.Fatalf("new state store: %v", err)
	}

	logPath := filepath.Join(store.LogsDir(), "guard-test.log")
	if err := store.UseLogPath(logPath); err != nil {
		t.Fatalf("use log path: %v", err)
	}
	if got := store.CurrentLogPath(); got != logPath {
		t.Fatalf("unexpected current log path: %q", got)
	}
	if got := store.CurrentEventPath(); got != store.EventPathForLog(logPath) {
		t.Fatalf("unexpected current event path: %q", got)
	}

	status := Status{
		Running:        true,
		Health:         HealthHealthy,
		DesiredProfile: "B",
		ScheduleWindow: "day",
		Binding:        BindingStatus{Audited: true, OK: true, Message: "binding ok"},
		Connectivity:   ConnectivityStatus{InitialOK: true, FinalOK: true},
		Portal:         PortalStatus{Attempted: false, OK: true, Message: "portal login not needed"},
		Cycle:          CycleStatus{Index: 1, RecoveryStep: "healthy"},
		Timing:         TimingStatus{Timestamp: "2026-03-19 18:00:00", ElapsedSeconds: 0.4},
		Log:            LogStatus{Path: logPath},
	}
	if err := store.WriteStatus(status); err != nil {
		t.Fatalf("write status: %v", err)
	}
	readStatus, err := store.ReadStatus()
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if readStatus.Health != HealthHealthy || readStatus.Log.Path != logPath {
		t.Fatalf("unexpected status roundtrip: %#v", readStatus)
	}

	if err := store.WriteStopRequest("unit-test stop"); err != nil {
		t.Fatalf("write stop request: %v", err)
	}
	reason, ok := store.ReadStopRequest()
	if !ok || reason != "unit-test stop" {
		t.Fatalf("unexpected stop request: reason=%q ok=%v", reason, ok)
	}

	if err := os.WriteFile(store.StopRequestFile(), []byte("{"), 0o644); err != nil {
		t.Fatalf("write malformed stop request: %v", err)
	}
	reason, ok = store.ReadStopRequest()
	if !ok || reason != "" {
		t.Fatalf("expected malformed stop request fallback, got reason=%q ok=%v", reason, ok)
	}
}

func TestSupervisorHelpersAndWindowsDefaults(t *testing.T) {
	args := BuildRunArgs([]string{"--config", "credentials.json"}, []string{"--state-dir", "dist/guard"})
	wantArgs := []string{"--config", "credentials.json", "guard", "run", "--state-dir", "dist/guard"}
	if len(args) != len(wantArgs) {
		t.Fatalf("unexpected args length: %#v", args)
	}
	for index := range wantArgs {
		if args[index] != wantArgs[index] {
			t.Fatalf("unexpected arg at %d: got %q want %q", index, args[index], wantArgs[index])
		}
	}

	var stdout bytes.Buffer
	logPath := filepath.Join(t.TempDir(), "foreground.log")
	writer, closeFn, err := OpenForegroundWriter(logPath, &stdout)
	if err != nil {
		t.Fatalf("open foreground writer: %v", err)
	}
	if _, err := writer.Write([]byte("guard-line")); err != nil {
		t.Fatalf("write foreground log: %v", err)
	}
	closeFn()

	logPayload, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read foreground log: %v", err)
	}
	if !strings.Contains(stdout.String(), "guard-line") || !strings.Contains(string(logPayload), "guard-line") {
		t.Fatalf("expected mirrored foreground output, stdout=%q log=%q", stdout.String(), string(logPayload))
	}

	if writer, closeFn, err = OpenForegroundWriter("", &stdout); err != nil || writer == nil || closeFn == nil {
		t.Fatalf("expected stdout-only foreground writer, got writer-nil=%v close-nil=%v err=%v", writer == nil, closeFn == nil, err)
	}
	closeFn()

	if !defaultProcessExists(os.Getpid()) {
		t.Fatal("expected current process to exist")
	}
	if defaultProcessExists(999999) {
		t.Fatal("did not expect pid 999999 to exist")
	}
	if _, err := defaultFindLegacyPIDs(); err != nil {
		t.Fatalf("defaultFindLegacyPIDs should not fail: %v", err)
	}
	if detachedSysProcAttr() == nil {
		t.Fatal("expected detached sysproc attr")
	}
}

func TestEventNormalizationCoversTypedPointerAndUnsupportedPaths(t *testing.T) {
	cases := []Event{
		{Kind: EventStartup, Details: &StartupEventDetails{StateDir: "dist/guard"}},
		{Kind: EventScheduleSwitch, Details: &ScheduleSwitchEventDetails{BindingOK: true, InternetOK: true, PortalLoginOK: false, RecoveryStep: "portal-login"}},
		{Kind: EventBindingAudit, Details: &BindingAuditEventDetails{BindingOK: true, RecoveryStep: "healthy"}},
		{Kind: EventPortalLogin, Details: &PortalLoginEventDetails{InternetOK: true, PortalLoginOK: true, RecoveryStep: "portal-login"}},
		{Kind: EventBindingRepair, Details: &BindingRepairEventDetails{Action: "moved", BindingOK: true, HolderProfile: "W", RecoveryStep: "binding-repair-then-portal-login", TargetProfile: "B"}},
		{Kind: EventDegraded, Details: &DegradedEventDetails{BindingOK: false, Error: "still offline", InternetOK: false, PortalLoginOK: false, RecoveryStep: "binding-repair-then-portal-login"}},
		{Kind: EventShutdown, Details: &ShutdownEventDetails{Reason: "unit-test"}},
		{Kind: EventFatal, Details: &FatalEventDetails{Error: "panic"}},
		{Kind: EventPortalLogin, Details: 17},
	}

	for _, tc := range cases {
		event := NormalizeEvent(tc)
		if _, err := json.Marshal(event); err != nil {
			t.Fatalf("marshal normalized event %s: %v", tc.Kind, err)
		}
	}
}

func TestRunnerOnceReturnsTypedStatus(t *testing.T) {
	store, err := NewStateStore(t.TempDir())
	if err != nil {
		t.Fatalf("new state store: %v", err)
	}
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
			PortalLoginMessage: "portal login not needed",
			RecoveryStep:       "healthy",
		}, nil
	}

	status, err := runner.Once(context.Background(), false)
	if err != nil {
		t.Fatalf("runner once: %v", err)
	}
	if status.Health != HealthHealthy || status.Cycle.RecoveryStep != "healthy" {
		t.Fatalf("unexpected once status: %#v", status)
	}
}
