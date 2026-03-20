package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_UsesEnvAndDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	payload := `{
	  "accounts": {
	    "A": {"username": "user-a", "password": "pass-a"}
	  },
	  "cmcc": {"account": "cmcc-user", "password": "cmcc-pass"},
	  "self": {},
	  "portal": {},
	  "guard": {
	    "schedule": {
	      "dayProfile": "A",
	      "nightProfile": "A"
	    }
	  }
	}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("NJUPT_NET_CONFIG", path)
	t.Setenv("NJUPT_NET_OUTPUT", "json")
	t.Setenv("NJUPT_NET_PORTAL_ISP", "telecom")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Output != "json" {
		t.Fatalf("expected env output override, got %q", cfg.Output)
	}
	if cfg.Self.BaseURL != defaultSelfBaseURL {
		t.Fatalf("expected default self base url, got %q", cfg.Self.BaseURL)
	}
	if cfg.Portal.BaseURL != defaultPortalBaseURL {
		t.Fatalf("expected default portal base url, got %q", cfg.Portal.BaseURL)
	}
	if cfg.Portal.ISP != "telecom" {
		t.Fatalf("expected env ISP override, got %q", cfg.Portal.ISP)
	}
	if len(cfg.Portal.FallbackBaseURLs) != 0 {
		t.Fatalf("expected no default portal fallback, got %#v", cfg.Portal.FallbackBaseURLs)
	}
	if cfg.Guard.StateDir != filepath.Join("dist", "guard") {
		t.Fatalf("expected default guard state dir, got %q", cfg.Guard.StateDir)
	}
	if cfg.Guard.Schedule.DayProfile != "A" || cfg.Guard.Schedule.NightProfile != "A" {
		t.Fatalf("unexpected explicit guard profiles: %#v", cfg.Guard.Schedule)
	}
}

func TestResolveAccount(t *testing.T) {
	cfg := &Config{
		Accounts: map[string]AccountConfig{
			"A": {Username: "user-a", Password: "pass-a"},
			"B": {Username: "user-b", Password: "pass-b"},
		},
	}

	account, err := cfg.ResolveAccount("A", "", "")
	if err != nil {
		t.Fatalf("resolve profile: %v", err)
	}
	if account.Username != "user-a" || account.Password != "pass-a" {
		t.Fatalf("unexpected account: %#v", account)
	}

	explicit, err := cfg.ResolveAccount("", "override", "secret")
	if err != nil {
		t.Fatalf("resolve explicit: %v", err)
	}
	if explicit.Username != "override" || explicit.Password != "secret" {
		t.Fatalf("unexpected explicit account: %#v", explicit)
	}

	_, err = cfg.ResolveAccount("", "", "")
	if err == nil || !strings.Contains(err.Error(), "profile is required") {
		t.Fatalf("expected profile-required error, got %v", err)
	}
}

func TestLoad_AllowsAccountlessConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	payload := `{
	  "self": {"baseURL": "http://self.example"},
	  "portal": {"baseURL": "https://portal.example"},
	  "guard": {
	    "schedule": {
	      "dayProfile": "A",
	      "nightProfile": "A"
	    }
	  }
	}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Accounts) != 0 {
		t.Fatalf("expected no configured accounts, got %#v", cfg.Accounts)
	}

	_, err = cfg.ResolveAccount("", "", "")
	if err == nil || !strings.Contains(err.Error(), "no configured accounts") {
		t.Fatalf("expected account resolution failure, got %v", err)
	}
}

func TestLoad_RequiresExplicitGuardProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	payload := `{
	  "accounts": {
	    "A": {"username": "user-a", "password": "pass-a"}
	  },
	  "self": {"baseURL": "http://self.example"},
	  "portal": {"baseURL": "https://portal.example"}
	}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "guard.schedule.dayProfile is required") {
		t.Fatalf("expected explicit dayProfile validation error, got %v", err)
	}
}

func TestResolveBroadband(t *testing.T) {
	cfg := &Config{
		CMCC: BroadbandConfig{Account: "broadband-user", Password: "broadband-pass"},
	}

	broadband, err := cfg.ResolveBroadband()
	if err != nil {
		t.Fatalf("resolve broadband: %v", err)
	}
	if broadband.Account != "broadband-user" || broadband.Password != "broadband-pass" {
		t.Fatalf("unexpected broadband: %#v", broadband)
	}

	cfg.CMCC = BroadbandConfig{}
	if _, err := cfg.ResolveBroadband(); err == nil || !strings.Contains(err.Error(), "cmcc") {
		t.Fatalf("expected cmcc validation error, got %v", err)
	}
}
