package selfservice

import (
	"context"
	"testing"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

func TestGetPersonSanitizesSensitiveOutput(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != personListPath {
				t.Fatalf("unexpected path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token1"><input name="userName" value="alice"><input name="password" value="secret"><script>(function (user) { window.user = user || {}; })({"userPassword":"secret","userName":"alice","userRealName":"Alice","installDate":"2024-01-01","serviceDefault":{"defaultName":"本科生"}});</script></html>`)}, nil
		},
	})

	result, err := client.GetPerson(context.Background())
	if err != nil {
		t.Fatalf("get person: %v", err)
	}
	if result == nil || result.Data == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Data.RawHTML != "" {
		t.Fatalf("expected raw html to be stripped, got %q", result.Data.RawHTML)
	}
	if result.Raw != nil {
		t.Fatalf("expected raw capture to be omitted, got %#v", result.Raw)
	}
	if result.Level != kernel.EvidenceConfirmed {
		t.Fatalf("expected confirmed level, got %s", result.Level)
	}
	if got := result.Data.Fields["password"]; got != "" {
		t.Fatalf("expected password field to be sanitized, got %q", got)
	}
	if got := result.Data.Fields["userName"]; got != "alice" {
		t.Fatalf("expected non-sensitive field to remain, got %q", got)
	}
	if got := result.Data.Fields["userRealName"]; got != "Alice" {
		t.Fatalf("expected projected userRealName, got %q", got)
	}
	if got := result.Data.Fields["serviceDefaultName"]; got != "本科生" {
		t.Fatalf("expected projected service default name, got %q", got)
	}
}

func TestUpdateUserSecurityDryRunReturnsSanitizedState(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token1"><input name="oldPassword" value="old-secret"><input name="userName" value="alice"></html>`)}, nil
		},
	})

	result, err := client.UpdateUserSecurity(context.Background(), map[string]string{"newPassword": "next-secret"}, true)
	if err != nil {
		t.Fatalf("dry-run update: %v", err)
	}
	if result == nil || result.Data == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
	if got := result.Data.Fields["oldPassword"]; got != "" {
		t.Fatalf("expected oldPassword to be sanitized, got %q", got)
	}
	if result.Data.RawHTML != "" {
		t.Fatalf("expected raw html to be stripped, got %q", result.Data.RawHTML)
	}
}
