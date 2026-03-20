package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAndMustConfirm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	payload := `{
	  "accounts": {
	    "A": {"username": "user-a", "password": "pass-a"}
	  },
	  "self": {"baseURL": "http://self.example", "timeoutSeconds": 5},
	  "portal": {"baseURL": "https://portal.example", "timeoutSeconds": 3, "insecureTLS": true}
	}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx, err := Load(Options{ConfigPath: path, OutputMode: "json"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("load app context: %v", err)
	}
	if ctx.Renderer == nil || ctx.Config == nil {
		t.Fatalf("unexpected context: %#v", ctx)
	}
	if !ctx.InsecureTLS {
		t.Fatal("expected portal config to propagate insecure tls")
	}
	if err := ctx.MustConfirm("dangerous op"); err == nil || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("expected confirmation error, got %v", err)
	}

	ctx.AssumeYes = true
	if err := ctx.MustConfirm("dangerous op"); err != nil {
		t.Fatalf("expected confirmation bypass, got %v", err)
	}
}
