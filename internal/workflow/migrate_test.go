package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

type mockSessionClient struct {
	getFn      func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error)
	postFormFn func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error)
}

func (m *mockSessionClient) Get(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
	if m.getFn != nil {
		return m.getFn(ctx, path, opts)
	}
	return nil, errors.New("mock get not implemented")
}

func (m *mockSessionClient) PostForm(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
	if m.postFormFn != nil {
		return m.postFormFn(ctx, path, opts)
	}
	return nil, errors.New("mock post not implemented")
}

func withSessionFactory(t *testing.T, factory func(string) (kernel.SessionClient, error)) {
	t.Helper()
	previous := newSessionClient
	newSessionClient = factory
	t.Cleanup(func() {
		newSessionClient = previous
	})
}

func TestMigrateBroadband_RejectsInvalidArgs(t *testing.T) {
	_, err := MigrateBroadband(context.Background(), "http://self.example", "", "p1", "u2", "p2", map[string]string{"FLDEXTRA1": "x"})
	if err == nil || !strings.Contains(err.Error(), "from/to credentials are required") {
		t.Fatalf("expected credential validation error, got: %v", err)
	}

	_, err = MigrateBroadband(context.Background(), "http://self.example", "u1", "p1", "u2", "p2", map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "target fields are required") {
		t.Fatalf("expected target field validation error, got: %v", err)
	}
}

func TestMigrateBroadband_FailFastOnSourceLogin(t *testing.T) {
	withSessionFactory(t, func(string) (kernel.SessionClient, error) {
		return &mockSessionClient{
			getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
				_ = ctx
				_ = path
				_ = opts
				return nil, errors.New("network down")
			},
		}, nil
	})

	_, err := MigrateBroadband(context.Background(), "http://self.example", "from", "frompwd", "to", "topwd", map[string]string{"FLDEXTRA1": "x"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "source login failed") {
		t.Fatalf("expected source login failure, got: %v", err)
	}
}

func TestMigrateBroadband_FailFastOnSourceUnbindReadback(t *testing.T) {
	operatorReadCount := 0
	withSessionFactory(t, func(string) (kernel.SessionClient, error) {
		return &mockSessionClient{
			getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
				_ = ctx
				_ = opts
				switch path {
				case "/Self/login/?302=LI":
					return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="checkcode" value="abcd"></html>`)}, nil
				case "/Self/login/randomCode":
					return &kernel.SessionResponse{StatusCode: 200, Body: []byte("ok")}, nil
				case "/Self/dashboard", "/Self/service":
					return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><div>protected page</div></html>`)}, nil
				case "/Self/service/operatorId":
					operatorReadCount++
					if operatorReadCount == 1 {
						return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token1"><input name="FLDEXTRA1" value="old1"><input name="FLDEXTRA2" value="old2"><input name="FLDEXTRA3" value="old3"><input name="FLDEXTRA4" value="old4"></html>`)}, nil
					}
					return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token2"><input name="FLDEXTRA1" value="still-old"><input name="FLDEXTRA2" value="old2"><input name="FLDEXTRA3" value="old3"><input name="FLDEXTRA4" value="old4"></html>`)}, nil
				default:
					return nil, errors.New("unexpected get path: " + path)
				}
			},
			postFormFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
				_ = ctx
				_ = opts
				switch path {
				case "/Self/login/verify":
					return &kernel.SessionResponse{StatusCode: 200, FinalURL: "/Self/dashboard"}, nil
				case "/Self/service/bind-operator":
					return &kernel.SessionResponse{StatusCode: 200, Body: []byte("ignored")}, nil
				default:
					return nil, errors.New("unexpected post path: " + path)
				}
			},
		}, nil
	})

	result, err := MigrateBroadband(context.Background(), "http://self.example", "from", "frompwd", "to", "topwd", map[string]string{"FLDEXTRA1": "target"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "readback mismatch") {
		t.Fatalf("expected readback failure, got: %v", err)
	}
	if result == nil || result.Data == nil || result.Data.SourceClear == nil {
		t.Fatalf("expected partial migrate result, got %#v", result)
	}
}

func TestMigrateBroadband_Succeeds(t *testing.T) {
	factoryCalls := 0
	withSessionFactory(t, func(string) (kernel.SessionClient, error) {
		factoryCalls++
		if factoryCalls == 1 {
			return successfulSession("source-old", "source-pass", map[string]string{
				"FLDEXTRA1": "old-telecom",
				"FLDEXTRA2": "old-pass",
				"FLDEXTRA3": "old-mobile",
				"FLDEXTRA4": "old-mobile-pass",
			}, map[string]string{
				"FLDEXTRA1": "",
				"FLDEXTRA2": "",
				"FLDEXTRA3": "",
				"FLDEXTRA4": "",
			}), nil
		}
		return successfulSession("target-new", "target-pass", map[string]string{
			"FLDEXTRA1": "",
			"FLDEXTRA2": "",
			"FLDEXTRA3": "",
			"FLDEXTRA4": "",
		}, map[string]string{
			"FLDEXTRA1": "target-mobile",
			"FLDEXTRA4": "target-secret",
		}), nil
	})

	result, err := MigrateBroadband(context.Background(), "http://self.example", "source-old", "source-pass", "target-new", "target-pass", map[string]string{
		"FLDEXTRA1": "target-mobile",
		"FLDEXTRA4": "target-secret",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result == nil || !result.Success || result.Data == nil || result.Data.TargetBind == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestMigrateBroadband_TargetBindFailure(t *testing.T) {
	factoryCalls := 0
	withSessionFactory(t, func(string) (kernel.SessionClient, error) {
		factoryCalls++
		if factoryCalls == 1 {
			return successfulSession("source-old", "source-pass", map[string]string{
				"FLDEXTRA1": "old-telecom",
				"FLDEXTRA2": "old-pass",
				"FLDEXTRA3": "old-mobile",
				"FLDEXTRA4": "old-mobile-pass",
			}, map[string]string{
				"FLDEXTRA1": "",
				"FLDEXTRA2": "",
				"FLDEXTRA3": "",
				"FLDEXTRA4": "",
			}), nil
		}
		return successfulSession("target-new", "target-pass", map[string]string{
			"FLDEXTRA1": "",
			"FLDEXTRA2": "",
			"FLDEXTRA3": "",
			"FLDEXTRA4": "",
		}, map[string]string{
			"FLDEXTRA1": "",
			"FLDEXTRA4": "",
		}), nil
	})

	result, err := MigrateBroadband(context.Background(), "http://self.example", "source-old", "source-pass", "target-new", "target-pass", map[string]string{
		"FLDEXTRA1": "target-mobile",
		"FLDEXTRA4": "target-secret",
	})
	if err == nil {
		t.Fatal("expected target bind failure")
	}
	if !strings.Contains(err.Error(), "readback mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Success || result.Data == nil || result.Data.TargetBind == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func successfulSession(expectedUser, expectedPassword string, initialState, postState map[string]string) *mockSessionClient {
	operatorReadCount := 0
	return &mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			switch path {
			case "/Self/login/?302=LI":
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="checkcode" value="abcd"></html>`)}, nil
			case "/Self/login/randomCode":
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte("ok")}, nil
			case "/Self/dashboard", "/Self/service":
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><div>protected page</div></html>`)}, nil
			case "/Self/service/operatorId":
				operatorReadCount++
				if operatorReadCount == 1 {
					return &kernel.SessionResponse{StatusCode: 200, Body: []byte(operatorHTML("token1", initialState))}, nil
				}
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(operatorHTML("token2", mergeBindingState(initialState, postState)))}, nil
			default:
				return nil, errors.New("unexpected get path: " + path)
			}
		},
		postFormFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			switch path {
			case "/Self/login/verify":
				if opts.Form["account"] != expectedUser || opts.Form["password"] != expectedPassword {
					return nil, errors.New("unexpected login credentials")
				}
				return &kernel.SessionResponse{StatusCode: 200, FinalURL: "/Self/dashboard"}, nil
			case "/Self/service/bind-operator":
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte("ok")}, nil
			default:
				return nil, errors.New("unexpected post path: " + path)
			}
		},
	}
}

func operatorHTML(token string, state map[string]string) string {
	return `<html><input name="csrftoken" value="` + token + `"><input name="FLDEXTRA1" value="` + state["FLDEXTRA1"] + `"><input name="FLDEXTRA2" value="` + state["FLDEXTRA2"] + `"><input name="FLDEXTRA3" value="` + state["FLDEXTRA3"] + `"><input name="FLDEXTRA4" value="` + state["FLDEXTRA4"] + `"></html>`
}

func mergeBindingState(base, updates map[string]string) map[string]string {
	out := map[string]string{
		"FLDEXTRA1": base["FLDEXTRA1"],
		"FLDEXTRA2": base["FLDEXTRA2"],
		"FLDEXTRA3": base["FLDEXTRA3"],
		"FLDEXTRA4": base["FLDEXTRA4"],
	}
	for key, value := range updates {
		out[key] = value
	}
	return out
}
