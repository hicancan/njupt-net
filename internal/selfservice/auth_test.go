package selfservice

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

func TestLoginSuccess(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			switch path {
			case loginPath:
				return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "login_page.html")}, nil
			case randomCodePath:
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte("ok")}, nil
			case dashboardPath, servicePath:
				return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "dashboard_page.html")}, nil
			default:
				t.Fatalf("unexpected get path: %s", path)
				return nil, nil
			}
		},
		postFormFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			if path != verifyPath {
				t.Fatalf("unexpected post path: %s", path)
			}
			if opts.Form["account"] != "alice" || opts.Form["password"] != "secret" {
				t.Fatalf("unexpected verify form: %#v", opts.Form)
			}
			return &kernel.SessionResponse{StatusCode: 200, FinalURL: "/Self/dashboard"}, nil
		},
	})

	result, err := client.Login(context.Background(), "alice", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result == nil || !result.Success || result.Data == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !result.Data.SessionAlive || !result.Data.DashboardReadable {
		t.Fatalf("unexpected login data: %#v", result.Data)
	}
}

func TestLoginRejectsMissingCheckcode(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><form></form></html>`)}, nil
		},
	})

	result, err := client.Login(context.Background(), "alice", "secret")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing checkcode token") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Success {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestStatusDetectsLoggedOutSession(t *testing.T) {
	loginHTML := `<html><input name="checkcode" value="abcd"><input name="account" value=""></html>`
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			switch path {
			case dashboardPath, servicePath:
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(loginHTML)}, nil
			default:
				t.Fatalf("unexpected get path: %s", path)
				return nil, nil
			}
		},
	})

	result, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if result == nil || result.Data == nil || result.Data.LoggedIn {
		t.Fatalf("unexpected status result: %#v", result)
	}
}

func TestLogoutVerifiesLoggedOutState(t *testing.T) {
	statusReads := 0
	resetCalled := false
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			switch path {
			case logoutPath:
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte("ok")}, nil
			case dashboardPath, servicePath:
				statusReads++
				return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "login_page.html")}, nil
			default:
				t.Fatalf("unexpected get path: %s", path)
				return nil, nil
			}
		},
		resetFn: func() error {
			resetCalled = true
			return nil
		},
	})

	result, err := client.Logout(context.Background())
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	if result == nil || result.Data == nil || result.Data.LoggedIn {
		t.Fatalf("unexpected logout result: %#v", result)
	}
	if !resetCalled {
		t.Fatal("expected session reset after logout")
	}
}

func TestLogoutVerificationFailure(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			switch path {
			case logoutPath:
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte("ok")}, nil
			case dashboardPath, servicePath:
				return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "dashboard_page.html")}, nil
			default:
				t.Fatalf("unexpected get path: %s", path)
				return nil, nil
			}
		},
	})

	result, err := client.Logout(context.Background())
	if err == nil {
		t.Fatal("expected logout verification failure")
	}
	if result == nil || result.Success {
		t.Fatalf("unexpected logout failure result: %#v", result)
	}
}

func TestLogoutReturnsGuardedSuccessWhenRedirectSuggestsLogout(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			switch path {
			case logoutPath:
				return &kernel.SessionResponse{StatusCode: 302, FinalURL: "http://10.10.244.240:8080/Self/"}, nil
			case dashboardPath, servicePath:
				return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "dashboard_page.html")}, nil
			default:
				t.Fatalf("unexpected get path: %s", path)
				return nil, nil
			}
		},
	})

	result, err := client.Logout(context.Background())
	if err != nil {
		t.Fatalf("expected guarded success, got %v", err)
	}
	if result == nil || !result.Success || result.Level != kernel.EvidenceGuarded {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestLogoutResetFailure(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != logoutPath {
				t.Fatalf("unexpected get path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 302, FinalURL: "/Self/"}, nil
		},
		resetFn: func() error {
			return errors.New("jar reset failed")
		},
	})

	result, err := client.Logout(context.Background())
	if err == nil {
		t.Fatal("expected reset failure")
	}
	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if !strings.Contains(err.Error(), "reset session cookies failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractLoginErrorMessage(t *testing.T) {
	body := []byte(`<html><div class="alert-danger">账号或密码错误</div></html>`)
	message := extractLoginErrorMessage(body)
	if !strings.Contains(message, "错误") {
		t.Fatalf("unexpected login error message: %q", message)
	}
}

func TestExtractLoginErrorMessageFallsBackToGeneric(t *testing.T) {
	message := extractLoginErrorMessage([]byte(`<html><div>plain text only</div></html>`))
	if message != "login failed" {
		t.Fatalf("unexpected fallback message: %q", message)
	}
}
