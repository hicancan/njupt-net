package portal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

type mockSessionClient struct {
	getFn      func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error)
	postJSONFn func(ctx context.Context, path string, opts kernel.RequestOptions, payload []byte) (*kernel.SessionResponse, error)
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

func (m *mockSessionClient) PostJSON(ctx context.Context, path string, opts kernel.RequestOptions, payload []byte) (*kernel.SessionResponse, error) {
	if m.postJSONFn != nil {
		return m.postJSONFn(ctx, path, opts, payload)
	}
	return nil, errors.New("mock post json not implemented")
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

func TestLogin802AC999ReturnsGuardedSuccess(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = path
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`dr1003({"result":0,"msg":"AC999","ret_code":2});`)}, nil
		},
	}, "https://portal.example", "")

	result, err := client.Login802(context.Background(), "user", "pass", "10.0.0.1", "mobile")
	if err != nil {
		t.Fatalf("expected guarded success, got %v", err)
	}
	if result == nil || !result.Success || result.Level != kernel.EvidenceGuarded {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Message != "portal 802 reports already online (AC999)" {
		t.Fatalf("unexpected message: %#v", result)
	}
}

func TestLogin802AggregatesTransportFailuresAcrossEndpoints(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			return nil, errors.New("dial failed")
		},
	}, "https://10.10.244.11:802/eportal/portal", "https://p.njupt.edu.cn:802/eportal/portal")

	result, err := client.Login802(context.Background(), "user", "pass", "10.0.0.1", "mobile")
	if err == nil {
		t.Fatal("expected transport failure")
	}
	if result != nil {
		t.Fatalf("expected nil result on transport aggregate failure, got %#v", result)
	}
	if !errors.Is(err, kernel.ErrPortal) {
		t.Fatalf("expected ErrPortal classification, got %v", err)
	}

	problems := kernel.ProblemsFromError(err)
	if len(problems) != 1 {
		t.Fatalf("unexpected problems: %#v", problems)
	}
	if problems[0].Code != kernel.ProblemPortalRequestFailed {
		t.Fatalf("unexpected problem code: %#v", problems[0])
	}
	details, ok := problems[0].Details.(kernel.PortalProblemDetails)
	if !ok {
		t.Fatalf("expected typed portal details, got %#v", problems[0].Details)
	}
	if len(details.Attempts) != 2 {
		t.Fatalf("expected 2 attempted endpoints, got %#v", details)
	}
	if details.Attempts[0].Endpoint == "" || details.Attempts[1].Endpoint == "" {
		t.Fatalf("expected endpoint list in details, got %#v", details)
	}
	if !strings.Contains(err.Error(), "portal 802 transport attempts failed") {
		t.Fatalf("unexpected aggregate message: %v", err)
	}
}

func TestNewClientLeavesFallbackEmptyWhenNotConfigured(t *testing.T) {
	client := NewClient(&mockSessionClient{}, "https://10.10.244.11:802/eportal/portal", "")
	if client.fallbackBaseURL802 != "" {
		t.Fatalf("expected empty fallback endpoint, got %q", client.fallbackBaseURL802)
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

func TestLogin801ReturnsBlockedAdminConsoleResultWhenTokenMissing(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = path
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<!DOCTYPE html><html><head><title>EPortal</title></head><body><div id=app></div><script src=/eportal/public/static/js/app.bb55e182.js></script></body></html>`)}, nil
		},
		postJSONFn: func(ctx context.Context, path string, opts kernel.RequestOptions, payload []byte) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != "http://p.njupt.edu.cn:801/eportal/admin/login/login" {
				t.Fatalf("unexpected path: %s", path)
			}
			if string(payload) != `{"username":"user","password":"1a1dc91c907325c69271ddf0c944bc72"}` {
				t.Fatalf("unexpected payload: %s", payload)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`{"code":0,"msg":"您已登录失败1次，登录失败超过5次账号将被锁定！失败次数隔天清零。"}`)}, nil
		},
	}, "https://portal.example", "")

	result, err := client.Login801(context.Background(), "user", "pass")
	if err == nil {
		t.Fatal("expected blocked admin-console error")
	}
	if !errors.Is(err, kernel.ErrBlockedCapability) {
		t.Fatalf("expected blocked-capability error, got %v", err)
	}
	if result == nil || result.Level != kernel.EvidenceBlocked {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Data == nil || !result.Data.AdminConsoleDetected || result.Data.TokenPresent {
		t.Fatalf("unexpected 801 response data: %#v", result.Data)
	}
	if !strings.Contains(result.Message, "returned no token") {
		t.Fatalf("unexpected message: %#v", result)
	}
}

func TestLogin801ReturnsConfirmedSuccessWhenTokenPresent(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = path
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<!DOCTYPE html><html><head><title>EPortal</title></head><body><div id=app></div></body></html>`)}, nil
		},
		postJSONFn: func(ctx context.Context, path string, opts kernel.RequestOptions, payload []byte) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = path
			_ = opts
			_ = payload
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`{"code":0,"msg":"ok","data":{"token":"abc","changepass":true}}`)}, nil
		},
	}, "https://portal.example", "")

	result, err := client.Login801(context.Background(), "user", "pass")
	if err != nil {
		t.Fatalf("expected confirmed success, got %v", err)
	}
	if result == nil || result.Level != kernel.EvidenceConfirmed || !result.Success || result.Data == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !result.Data.TokenPresent || !result.Data.ChangePass {
		t.Fatalf("expected token/changePass markers, got %#v", result.Data)
	}
}

func TestLogout801ReturnsConfirmedSuccess(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = path
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte("<html><body>Logout succeed.</body></html>")}, nil
		},
	}, "https://portal.example", "")

	result, err := client.Logout801(context.Background(), "10.0.0.1")
	if err != nil {
		t.Fatalf("expected confirmed success, got %v", err)
	}
	if result == nil || result.Level != kernel.EvidenceConfirmed || !result.Success {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestLogout801ReturnsGuardedFallbackWithoutSuccessMarker(t *testing.T) {
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
