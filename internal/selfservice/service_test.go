package selfservice

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

func TestBindOperatorReadbackSuccess(t *testing.T) {
	readCount := 0
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != operatorIDPath {
				t.Fatalf("unexpected get path: %s", path)
			}
			readCount++
			if readCount == 1 {
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token1"><input name="FLDEXTRA1" value="old1"><input name="FLDEXTRA2" value="old2"><input name="FLDEXTRA3" value=""><input name="FLDEXTRA4" value=""></html>`)}, nil
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token2"><input name="FLDEXTRA1" value="new1"><input name="FLDEXTRA2" value="old2"><input name="FLDEXTRA3" value=""><input name="FLDEXTRA4" value=""></html>`)}, nil
		},
		postFormFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			if path != bindOperatorPath {
				t.Fatalf("unexpected post path: %s", path)
			}
			if opts.Form["FLDEXTRA1"] != "new1" {
				t.Fatalf("unexpected bind form: %#v", opts.Form)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte("ok")}, nil
		},
	})

	result, err := client.BindOperator(context.Background(), map[string]string{"FLDEXTRA1": "new1"}, true, false)
	if err != nil {
		t.Fatalf("bind operator: %v", err)
	}
	if result == nil || !result.Success || result.Data == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Data.PostState["FLDEXTRA1"] != "new1" {
		t.Fatalf("unexpected post state: %#v", result.Data.PostState)
	}
}

func TestBindOperatorRestoreSuccess(t *testing.T) {
	readCount := 0
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			readCount++
			switch readCount {
			case 1:
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token1"><input name="FLDEXTRA1" value="old1"><input name="FLDEXTRA2" value="old2"><input name="FLDEXTRA3" value=""><input name="FLDEXTRA4" value=""></html>`)}, nil
			case 2:
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token2"><input name="FLDEXTRA1" value="new1"><input name="FLDEXTRA2" value="old2"><input name="FLDEXTRA3" value=""><input name="FLDEXTRA4" value=""></html>`)}, nil
			default:
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token3"><input name="FLDEXTRA1" value="old1"><input name="FLDEXTRA2" value="old2"><input name="FLDEXTRA3" value=""><input name="FLDEXTRA4" value=""></html>`)}, nil
			}
		},
		postFormFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = path
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte("ok")}, nil
		},
	})

	result, err := client.BindOperator(context.Background(), map[string]string{"FLDEXTRA1": "new1"}, true, true)
	if err != nil {
		t.Fatalf("bind operator restore: %v", err)
	}
	if result == nil || !result.Success || result.Data == nil || result.Data.RestoredState["FLDEXTRA1"] != "old1" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestGetOperatorBinding(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "binding_state.html")}, nil
		},
	})

	result, err := client.GetOperatorBinding(context.Background())
	if err != nil {
		t.Fatalf("get operator binding: %v", err)
	}
	if result == nil || result.Data == nil || result.Data.MobileAccount != "mob" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestBindOperatorPrefersBusinessFailureMessage(t *testing.T) {
	readCount := 0
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			readCount++
			if readCount == 1 {
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token1"><input name="FLDEXTRA1" value=""><input name="FLDEXTRA2" value=""><input name="FLDEXTRA3" value=""><input name="FLDEXTRA4" value=""></html>`)}, nil
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token2"><input name="FLDEXTRA1" value=""><input name="FLDEXTRA2" value=""><input name="FLDEXTRA3" value=""><input name="FLDEXTRA4" value=""></html>`)}, nil
		},
		postFormFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = path
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><div class="swal2-content">中国移动账号:13800138000,绑定失败,已存在该运营商账号</div></html>`)}, nil
		},
	})

	result, err := client.BindOperator(context.Background(), map[string]string{"FLDEXTRA3": "13800138000"}, true, false)
	if err == nil {
		t.Fatal("expected business failure")
	}
	if !errors.Is(err, kernel.ErrBusinessFailed) {
		t.Fatalf("expected business failure classification, got %v", err)
	}
	if result == nil || result.Success {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !strings.Contains(result.Message, "已存在该运营商账号") {
		t.Fatalf("expected business message, got %#v", result)
	}
}

func TestExtractBusinessMessage(t *testing.T) {
	body := []byte(`<html><div class="layui-layer-content">中国移动账号 B24040213@cmcc 绑定失败,已存在该运营商账号</div></html>`)
	got := extractBusinessMessage(body)
	if !strings.Contains(got, "已存在该运营商账号") {
		t.Fatalf("unexpected business message: %q", got)
	}
}

func TestExtractBusinessMessageIgnoresInlineScripts(t *testing.T) {
	body := []byte(`<html><script>$(function(){var exit="注销登录";var recharge="账户充值";});</script><input name="FLDEXTRA3" value="13800138000"></html>`)
	got := extractBusinessMessage(body)
	if got != "" {
		t.Fatalf("expected no business message, got %q", got)
	}
}

func TestExtractBusinessMessageReadsSweetAlertV1Body(t *testing.T) {
	body := []byte(`<html><div class="sweet-alert"><h2>提示</h2><p>中国移动账号:13800138000,绑定失败,请核对运营商账号的密码是否正确。</p><button>确定</button></div></html>`)
	got := extractBusinessMessage(body)
	if !strings.Contains(got, "请核对运营商账号的密码是否正确") {
		t.Fatalf("unexpected business message: %q", got)
	}
	if strings.Contains(got, "确定") {
		t.Fatalf("expected only alert body, got %q", got)
	}
}

func TestBindOperatorPrefersBusinessFailureWithoutReadback(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token1"><input name="FLDEXTRA1" value=""><input name="FLDEXTRA2" value=""><input name="FLDEXTRA3" value=""><input name="FLDEXTRA4" value=""></html>`)}, nil
		},
		postFormFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = path
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><div class="swal2-content">中国移动账号:13800138000,绑定失败,已存在该运营商账号</div></html>`)}, nil
		},
	})

	result, err := client.BindOperator(context.Background(), map[string]string{"FLDEXTRA3": "13800138000"}, false, false)
	if err == nil {
		t.Fatal("expected business failure")
	}
	if !errors.Is(err, kernel.ErrBusinessFailed) {
		t.Fatalf("expected business failure classification, got %v", err)
	}
	if result == nil || result.Success {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestBindOperatorRejectsEmptyTarget(t *testing.T) {
	client := NewClient(&mockSessionClient{})

	_, err := client.BindOperator(context.Background(), map[string]string{}, false, false)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errors.Is(err, kernel.ErrBusinessFailed) {
		t.Fatalf("expected business_failed classification, got %v", err)
	}
}

func TestChangeConsumeProtectReadbackMismatch(t *testing.T) {
	readCount := 0
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != consumeProtectPath {
				t.Fatalf("unexpected get path: %s", path)
			}
			readCount++
			if readCount == 1 {
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token1"><input name="consumeLimit" value="40"></html><script>var installmentFlag="40";</script>`)}, nil
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token2"><input name="consumeLimit" value="40"></html><script>var installmentFlag="40";</script>`)}, nil
		},
		postFormFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			if path != changeConsumePath {
				t.Fatalf("unexpected post path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte("ok")}, nil
		},
	})

	result, err := client.ChangeConsumeProtect(context.Background(), "80", true, false)
	if err == nil {
		t.Fatal("expected readback mismatch error")
	}
	if !strings.Contains(err.Error(), "readback mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Success {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestGetConsumeProtectParsesInstallmentFlag(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != consumeProtectPath {
				t.Fatalf("unexpected get path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "consume_protect.html")}, nil
		},
	})

	result, err := client.GetConsumeProtect(context.Background())
	if err != nil {
		t.Fatalf("get consume protect: %v", err)
	}
	if result == nil || result.Data == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Data.InstallmentFlag != "80" || result.Data.CurrentUsage != "12" || result.Data.Balance != "99" {
		t.Fatalf("unexpected consume state: %#v", result.Data)
	}
}

func TestGetConsumeProtectRejectsLoginPage(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "login_page.html")}, nil
		},
	})

	_, err := client.GetConsumeProtect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "returned login page") {
		t.Fatalf("expected login page error, got %v", err)
	}
}

func TestGetMacList(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path == "/Self/service/myMac" {
				return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "dashboard_page.html")}, nil
			}
			if path != macListPath {
				t.Fatalf("unexpected path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`{"total":1,"rows":[{"mac":"aa:bb"}]}`)}, nil
		},
	})

	result, err := client.GetMacList(context.Background())
	if err != nil {
		t.Fatalf("get mac list: %v", err)
	}
	if result == nil || result.Data == nil || result.Data.Total != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestGetMacListAcceptsArrayRows(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path == "/Self/service/myMac" {
				return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "dashboard_page.html")}, nil
			}
			if path != macListPath {
				t.Fatalf("unexpected path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`{"total":1,"rows":[["aa:bb","device"]]}`)}, nil
		},
	})

	result, err := client.GetMacList(context.Background())
	if err != nil {
		t.Fatalf("get mac list: %v", err)
	}
	if result == nil || result.Data == nil || result.Data.Total != 1 || len(result.Data.Rows) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestGetMacListEmptyBodyTreatsAsZeroRows(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path == "/Self/service/myMac" {
				return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "dashboard_page.html")}, nil
			}
			if path != macListPath {
				t.Fatalf("unexpected path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte("")}, nil
		},
	})

	result, err := client.GetMacList(context.Background())
	if err != nil {
		t.Fatalf("get mac list empty body: %v", err)
	}
	if result == nil || result.Data == nil || result.Data.Total != 0 || len(result.Data.Rows) != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestGetMacListRejectsLoginPage(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path == "/Self/service/myMac" {
				return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "login_page.html")}, nil
			}
			t.Fatalf("unexpected path: %s", path)
			return nil, nil
		},
	})

	_, err := client.GetMacList(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !strings.Contains(err.Error(), "myMac returned login page") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChangeConsumeProtectRestoreSuccess(t *testing.T) {
	readCount := 0
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			readCount++
			switch readCount {
			case 1:
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token1"><input name="consumeLimit" value="40"></html><script>var installmentFlag="40";</script>`)}, nil
			case 2:
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token2"><input name="consumeLimit" value="80"></html><script>var installmentFlag="80";</script>`)}, nil
			default:
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`<html><input name="csrftoken" value="token3"><input name="consumeLimit" value="40"></html><script>var installmentFlag="40";</script>`)}, nil
			}
		},
		postFormFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = path
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte("ok")}, nil
		},
	})

	result, err := client.ChangeConsumeProtect(context.Background(), "80", true, true)
	if err != nil {
		t.Fatalf("change consume protect restore: %v", err)
	}
	if result == nil || !result.Success || result.Data == nil || result.Data.RestoredState["installmentFlag"] != "40" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
