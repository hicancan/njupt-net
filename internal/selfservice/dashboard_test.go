package selfservice

import (
	"context"
	"strings"
	"testing"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

func TestGetOnlineListParsesRows(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != "/Self/dashboard/getOnlineList" {
				t.Fatalf("unexpected path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "online_list.json")}, nil
		},
	})

	result, err := client.GetOnlineList(context.Background())
	if err != nil {
		t.Fatalf("get online list: %v", err)
	}
	if result == nil || result.Data == nil || len(*result.Data) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if (*result.Data)[0].SessionID != "sid-1" {
		t.Fatalf("unexpected session row: %#v", (*result.Data)[0])
	}
}

func TestGetLoginHistoryParsesRows(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != "/Self/dashboard/getLoginHistory" {
				t.Fatalf("unexpected path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "login_history.json")}, nil
		},
	})

	result, err := client.GetLoginHistory(context.Background())
	if err != nil {
		t.Fatalf("get login history: %v", err)
	}
	if result == nil || result.Data == nil || len(*result.Data) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if (*result.Data)[0].TerminalType != "phone" {
		t.Fatalf("unexpected history row: %#v", (*result.Data)[0])
	}
}

func TestToggleMauthSuccess(t *testing.T) {
	previousPause := mauthTogglePause
	mauthTogglePause = 0
	t.Cleanup(func() {
		mauthTogglePause = previousPause
	})

	stateReads := 0
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			switch path {
			case "/Self/dashboard/refreshMauthType":
				stateReads++
				if stateReads == 1 {
					return &kernel.SessionResponse{StatusCode: 200, Body: []byte("默认")}, nil
				}
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte("关闭")}, nil
			case "/Self/dashboard/oprateMauthAction":
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte("ok")}, nil
			default:
				t.Fatalf("unexpected path: %s", path)
				return nil, nil
			}
		},
	})

	result, err := client.ToggleMauth(context.Background())
	if err != nil {
		t.Fatalf("toggle mauth: %v", err)
	}
	if result == nil || !result.Success || result.Data == nil || *result.Data != kernel.MauthOff {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestGetMauthStateLoginPage(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			return &kernel.SessionResponse{StatusCode: 200, Body: fixture(t, "login_page.html")}, nil
		},
	})

	result, err := client.GetMauthState(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
	if result == nil || result.Success || result.Data == nil || *result.Data != kernel.MauthUnknown {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestToggleMauthUnknownState(t *testing.T) {
	previousPause := mauthTogglePause
	mauthTogglePause = 0
	t.Cleanup(func() {
		mauthTogglePause = previousPause
	})

	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			switch path {
			case "/Self/dashboard/refreshMauthType":
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte("未知")}, nil
			case "/Self/dashboard/oprateMauthAction":
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte("ok")}, nil
			default:
				t.Fatalf("unexpected path: %s", path)
				return nil, nil
			}
		},
	})

	_, err := client.ToggleMauth(context.Background())
	if err == nil {
		t.Fatal("expected guarded capability error")
	}
	if !strings.Contains(err.Error(), "current mauth state unknown") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestForceOfflineRequiresSessionPresence(t *testing.T) {
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			_ = opts
			if path != "/Self/dashboard/getOnlineList" {
				t.Fatalf("unexpected path: %s", path)
			}
			return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`[{"sessionId":"sid-1"}]`)}, nil
		},
	})

	result, err := client.ForceOffline(context.Background(), "sid-missing")
	if err == nil {
		t.Fatal("expected guarded error")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Success {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestForceOfflineSuccess(t *testing.T) {
	previousPause := offlineReadbackPause
	previousAttempts := offlineReadbackAttempts
	offlineReadbackPause = 0
	offlineReadbackAttempts = 3
	t.Cleanup(func() {
		offlineReadbackPause = previousPause
		offlineReadbackAttempts = previousAttempts
	})

	onlineReads := 0
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			switch path {
			case "/Self/dashboard/getOnlineList":
				onlineReads++
				if onlineReads == 1 {
					return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`[{"sessionId":"sid-1"}]`)}, nil
				}
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`[]`)}, nil
			case "/Self/dashboard/tooffline":
				if opts.Query["sessionid"] != "sid-1" {
					t.Fatalf("unexpected offline query: %#v", opts.Query)
				}
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`{"success":true}`)}, nil
			default:
				t.Fatalf("unexpected path: %s", path)
				return nil, nil
			}
		},
	})

	result, err := client.ForceOffline(context.Background(), "sid-1")
	if err != nil {
		t.Fatalf("force offline: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestForceOfflineTreatsReplacementSessionAsSuccess(t *testing.T) {
	previousPause := offlineReadbackPause
	previousAttempts := offlineReadbackAttempts
	offlineReadbackPause = 0
	offlineReadbackAttempts = 3
	t.Cleanup(func() {
		offlineReadbackPause = previousPause
		offlineReadbackAttempts = previousAttempts
	})

	onlineReads := 0
	client := NewClient(&mockSessionClient{
		getFn: func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
			_ = ctx
			switch path {
			case "/Self/dashboard/getOnlineList":
				onlineReads++
				switch onlineReads {
				case 1:
					return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`[{"sessionId":"sid-1"}]`)}, nil
				case 2:
					return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`[{"sessionId":"sid-1"},{"sessionId":"sid-2"}]`)}, nil
				default:
					return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`[{"sessionId":"sid-2"}]`)}, nil
				}
			case "/Self/dashboard/tooffline":
				return &kernel.SessionResponse{StatusCode: 200, Body: []byte(`{"success":true}`)}, nil
			default:
				t.Fatalf("unexpected path: %s", path)
				return nil, nil
			}
		},
	})

	result, err := client.ForceOffline(context.Background(), "sid-1")
	if err != nil {
		t.Fatalf("force offline replacement: %v", err)
	}
	if result == nil || !result.Success || result.Data == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
	if detected, ok := (*result.Data)["replacementSessionDetected"].(bool); !ok || !detected {
		t.Fatalf("expected replacement session marker, got %#v", result.Data)
	}
}

func TestOfflineSessionState(t *testing.T) {
	rows := []kernel.OnlineSession{
		{SessionID: "sid-1"},
		{SessionID: "sid-2"},
	}
	stillExists, replacementDetected := offlineSessionState(&rows, "sid-1")
	if !stillExists || !replacementDetected {
		t.Fatalf("unexpected state: still=%v replacement=%v", stillExists, replacementDetected)
	}

	rows = []kernel.OnlineSession{{SessionID: "sid-2"}}
	stillExists, replacementDetected = offlineSessionState(&rows, "sid-1")
	if stillExists || !replacementDetected {
		t.Fatalf("unexpected replacement-only state: still=%v replacement=%v", stillExists, replacementDetected)
	}

	stillExists, replacementDetected = offlineSessionState(nil, "sid-1")
	if stillExists || replacementDetected {
		t.Fatalf("unexpected empty state: still=%v replacement=%v", stillExists, replacementDetected)
	}
}
