package portal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

type mockSessionClient struct {
	getFn func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error)
}

func (m *mockSessionClient) Get(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
	if m.getFn != nil {
		return m.getFn(ctx, path, opts)
	}
	return nil, errors.New("mock get not implemented")
}

func (m *mockSessionClient) PostForm(context.Context, string, kernel.RequestOptions) (*kernel.SessionResponse, error) {
	return nil, errors.New("mock post not implemented")
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return payload
}

func TestParseJSONPPayload(t *testing.T) {
	payload, err := parseJSONPPayload(string(fixture(t, "login_success.jsonp")))
	if err != nil {
		t.Fatalf("parse jsonp: %v", err)
	}
	if payload["result"] != "1" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestLogin802Success(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != "https://portal.example/login" {
				t.Fatalf("unexpected path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "login_success.jsonp")}, nil
		},
	}, "https://portal.example", "")

	result, err := client.Login802(context.Background(), "user", "pass", "10.0.0.1", "mobile")
	if err != nil {
		t.Fatalf("login 802: %v", err)
	}
	if result == nil || !result.Success || result.Data == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Data.Endpoint != "https://portal.example/login" {
		t.Fatalf("unexpected endpoint: %#v", result.Data)
	}
}

func TestLogin802RetCode1ReturnsGuardedError(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = path
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "login_retcode1.jsonp")}, nil
		},
	}, "https://portal.example", "")

	result, err := client.Login802(context.Background(), "user", "pass", "10.0.0.1", "mobile")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, kernel.ErrPortalRetCode1) {
		t.Fatalf("expected ErrPortalRetCode1, got %v", err)
	}
	if result == nil || result.Level != kernel.EvidenceGuarded {
		t.Fatalf("unexpected guarded result: %#v", result)
	}
}

func TestLogout802Success(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != "https://portal.example/logout" {
				t.Fatalf("unexpected path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`dr1003({"result":"1"})`)}, nil
		},
	}, "https://portal.example", "")

	result, err := client.Logout802(context.Background(), "10.0.0.1")
	if err != nil {
		t.Fatalf("logout 802: %v", err)
	}
	if result == nil || !result.Success || result.Data == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestLogin801ReturnsGuardedFallback(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = path
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte("legacy body")}, nil
		},
	}, "https://portal.example", "")

	result, err := client.Login801(context.Background(), "user", "pass", "10.0.0.1", "")
	if err == nil {
		t.Fatal("expected guarded fallback error")
	}
	if !errors.Is(err, kernel.ErrPortalFallbackRequired) {
		t.Fatalf("expected fallback-required error, got %v", err)
	}
	if result == nil || result.Level != kernel.EvidenceGuarded {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestLogout801ReturnsGuardedFallback(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = path
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte("legacy logout")}, nil
		},
	}, "https://portal.example", "")

	result, err := client.Logout801(context.Background(), "10.0.0.1")
	if err == nil {
		t.Fatal("expected guarded fallback error")
	}
	if !errors.Is(err, kernel.ErrPortalFallbackRequired) {
		t.Fatalf("expected fallback-required error, got %v", err)
	}
	if result == nil || result.Level != kernel.EvidenceGuarded {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestISPSuffix(t *testing.T) {
	cases := map[string]string{
		"telecom": "@dx",
		"unicom":  "@lt",
		"mobile":  "@cmcc",
		"":        "",
	}
	for input, want := range cases {
		if got := ispSuffix(input); got != want {
			t.Fatalf("ispSuffix(%q)=%q want %q", input, got, want)
		}
	}
}

func TestParseJSONPPayloadRejectsInvalidBody(t *testing.T) {
	_, err := parseJSONPPayload(string(fixture(t, "invalid_jsonp.txt")))
	if err == nil {
		t.Fatal("expected invalid jsonp error")
	}
}

func TestClassifyRetCode(t *testing.T) {
	cases := map[string]error{
		"1": kernel.ErrPortalRetCode1,
		"3": kernel.ErrPortalRetCode3,
		"8": kernel.ErrPortalRetCode8,
		"9": kernel.ErrPortalUnknownCode,
		"":  kernel.ErrPortal,
	}
	for retCode, wantErr := range cases {
		_, err := classifyRetCode(retCode)
		if !errors.Is(err, wantErr) {
			t.Fatalf("classifyRetCode(%q)=%v want %v", retCode, err, wantErr)
		}
	}
}
