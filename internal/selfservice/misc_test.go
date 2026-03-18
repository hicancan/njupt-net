package selfservice

import (
	"context"
	"testing"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

func TestGetUserOnlineLogParsesRows(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != "/Self/bill/getUserOnlineLog" {
				t.Fatalf("unexpected path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "bill_online.json")}, nil
		},
	})

	result, err := client.GetUserOnlineLog(context.Background(), "", "")
	if err != nil {
		t.Fatalf("get user online log: %v", err)
	}
	if result == nil || result.Data == nil || result.Data.Total != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestGetMonthPayParsesRows(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != "/Self/bill/getMonthPay" {
				t.Fatalf("unexpected path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`{"summary":{"month":"2026-03"},"total":2,"rows":[{"month":"2026-03"},{"month":"2026-02"}]}`)}, nil
		},
	})

	result, err := client.GetMonthPay(context.Background(), "", "")
	if err != nil {
		t.Fatalf("get month pay: %v", err)
	}
	if result == nil || result.Data == nil || result.Data.Total != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestGetOperatorLogParsesRows(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != "/Self/bill/getOperatorLog" {
				t.Fatalf("unexpected path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`{"summary":{"rows":"1"},"total":1,"rows":[{"op":"bind"}]}`)}, nil
		},
	})

	result, err := client.GetOperatorLog(context.Background(), "", "")
	if err != nil {
		t.Fatalf("get operator log: %v", err)
	}
	if result == nil || result.Data == nil || result.Data.Total != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestUpdateUserSecurityDryRun(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token1"><input name="phone" value="13800000000"></html>`)}, nil
		},
	})

	result, err := client.UpdateUserSecurity(context.Background(), map[string]string{"phone": "13900000000"}, true)
	if err != nil {
		t.Fatalf("dry-run update user security: %v", err)
	}
	if result == nil || result.Level != kernel.EvidenceBlocked || result.Success {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestUpdateUserSecurityBlockedSubmit(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "person_list.html")}, nil
		},
		postFormFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			if path != updateUserSecurityPath {
				t.Fatalf("unexpected path: %s", path)
			}
			if opts.Form["phone"] != "13900000000" {
				t.Fatalf("unexpected form: %#v", opts.Form)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html>submitted</html>`)}, nil
		},
	})

	result, err := client.UpdateUserSecurity(context.Background(), map[string]string{"phone": "13900000000"}, false)
	if err == nil {
		t.Fatal("expected blocked capability error")
	}
	if result == nil || result.Level != kernel.EvidenceBlocked || result.Success {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestGetPersonLoadsGuardedState(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != personListPath {
				t.Fatalf("unexpected path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "person_list.html")}, nil
		},
	})

	result, err := client.GetPerson(context.Background())
	if err != nil {
		t.Fatalf("get person: %v", err)
	}
	if result == nil || result.Data == nil || result.Data.Fields["phone"] != "13800000000" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestGetPersonRejectsLoginPage(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "login_page.html")}, nil
		},
	})

	_, err := client.GetPerson(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestRefreshAccountRaw(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != "/Self/dashboard/refreshaccount" {
				t.Fatalf("unexpected path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte("refresh body")}, nil
		},
	})

	result, err := client.RefreshAccountRaw(context.Background())
	if err != nil {
		t.Fatalf("refresh account raw: %v", err)
	}
	if result == nil || result.Data == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRawGetCapturesResponse(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, FinalURL: path, Body: []byte("body")}, nil
		},
	})

	result, err := client.RawGet(context.Background(), "/probe")
	if err != nil {
		t.Fatalf("raw get: %v", err)
	}
	if result == nil || result.Raw == nil || result.Raw.Body != "body" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRawPostCapturesResponse(t *testing.T) {
	client := NewClient(&mockSessionClient{
		postFormFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			if path != "/probe" || opts.Form["a"] != "1" {
				t.Fatalf("unexpected raw post call: path=%s form=%#v", path, opts.Form)
			}
			return &kernel.SessionResponse{StatusCode: 200, FinalURL: path, Body: []byte("posted")}, nil
		},
	})

	result, err := client.RawPost(context.Background(), "/probe", map[string]string{"a": "1"})
	if err != nil {
		t.Fatalf("raw post: %v", err)
	}
	if result == nil || result.Raw == nil || result.Raw.Body != "posted" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
