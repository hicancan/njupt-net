package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
	"github.com/hicancan/njupt-net-cli/internal/selfservice"
)

func TestSelfDoctorSuccess(t *testing.T) {
	client := selfservice.NewClient(&mockSessionClient{
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
			default:
				return nil, errors.New("unexpected get path: " + path)
			}
		},
		postFormFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != "/Self/login/verify" {
				return nil, errors.New("unexpected post path: " + path)
			}
			return &kernel.SessionResponse{StatusCode: 200, FinalURL: "/Self/dashboard"}, nil
		},
	})

	result, err := SelfDoctor(context.Background(), client, "user", "pass")
	if err != nil {
		t.Fatalf("self doctor: %v", err)
	}
	if result == nil || !result.Success || result.Data == nil || result.Data.Status == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestSelfDoctorLoginFailure(t *testing.T) {
	client := selfservice.NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><form></form></html>`)}, nil
		},
	})

	result, err := SelfDoctor(context.Background(), client, "user", "pass")
	if err == nil {
		t.Fatal("expected login error")
	}
	if !strings.Contains(err.Error(), "missing checkcode token") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Success {
		t.Fatalf("unexpected result: %#v", result)
	}
}
