package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigRejectsMisspelledFieldAndTrailingValues(t *testing.T) {
	for _, data := range []string{
		`{"accounts":{"test":{"account":"x","password":"y","passwrod":"z"}}}`,
		`{"accounts":{}} {"accounts":{}}`,
		`null`,
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadConfig(path); err == nil {
			t.Fatalf("invalid config accepted: %s", data)
		}
	}
}

func TestCredentialRequiresExplicitAlias(t *testing.T) {
	cfg := config{Accounts: map[string]credential{"W": {Account: "account", Password: "password"}}}
	if _, err := cfg.credential(""); err == nil {
		t.Fatal("implicit account selection")
	}
	if got, err := cfg.credential("W"); err != nil || got.Account != "account" {
		t.Fatalf("credential: %v", err)
	}
}

func TestBridgeIdentityDoesNotRequireOrValidatePassword(t *testing.T) {
	cfg := config{Accounts: map[string]credential{"chosen": {Account: "expected-account"}}}
	if value, err := cfg.identity("chosen"); err != nil || value.Account != "expected-account" {
		t.Fatalf("bridge identity required an unused password: %v", err)
	}
	if _, err := cfg.credential("chosen"); err == nil {
		t.Fatal("password authentication accepted a missing password")
	}
}

func TestConfigPreservesSchemaAndCredentialText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{"accounts":{"W":{"account":"one","password":" space "},"B":{"account":"two","password":"second"}},"broadband_account":{"operator":"cmcc","account":"broadband","password":"third"}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	value, err := cfg.credential("W")
	if err != nil || value.Password != " space " || cfg.BroadbandAccount.Operator != "cmcc" || len(cfg.Accounts) != 2 {
		t.Fatalf("schema or credential text changed: %v", err)
	}
}
