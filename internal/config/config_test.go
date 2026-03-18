package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_UsesEnvAndDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	payload := `{
	  "accounts": {
	    "A": {"username": "user-a", "password": "pass-a"}
	  },
	  "cmcc": {"account": "cmcc-user", "password": "cmcc-pass"},
	  "self": {},
	  "portal": {}
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
	if len(cfg.Portal.FallbackBaseURLs) == 0 {
		t.Fatal("expected default portal fallback")
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
	path := filepath.Join(dir, "credentials.json")
	payload := `{
	  "self": {"baseURL": "http://self.example"},
	  "portal": {"baseURL": "https://portal.example"}
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
