package p

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const portalTestConfig = `{"code":1,"data":{"program_index":"fresh-program","page_index":"fresh-page","login_method":1,"check_online_method":"1","account_prefix":"1","ipad_terminal_identity":"0","no_filter_accandpwd":"0","enable_r3":0,"en_md5":0,"register_mode":"1","ac_logout":"1","rcn":"fresh-nonce"}}`

type portalFixture struct {
	mu                 sync.Mutex
	portal             *Portal
	requests           []string
	queries            map[string][]url.Values
	online             bool
	uid                string
	loginResponse      string
	logoutResponse     string
	config             string
	badCallback        bool
	loginNoStateChange bool
	statusResponse     string
	loginDelayQueries  int
	pendingOnline      bool
}

func newPortalFixture(t *testing.T, terminal string) *portalFixture {
	t.Helper()
	f := &portalFixture{queries: map[string][]url.Values{}, config: portalTestConfig, loginResponse: `{"result":1}`, logoutResponse: `{"result":"ok"}`}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests = append(f.requests, r.URL.Path)
		f.queries[r.URL.Path] = append(f.queries[r.URL.Path], r.URL.Query())
		switch r.URL.Path {
		case "/eportal/portal/captcha":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("test-image"))
			return
		}
		callback := r.URL.Query().Get("callback")
		if r.URL.Path == "/eportal/portal/page/loadConfig" || r.URL.Path == "/eportal/portal/online_list" {
			if query := r.URL.Query(); len(query) != 1 || len(query["callback"]) != 1 || callback == "" {
				t.Errorf("native query has unexpected parameters: %v", query)
			}
		}
		if f.badCallback {
			callback = "wrongCallback"
		}
		var response string
		switch r.URL.Path {
		case "/eportal/portal/page/loadConfig":
			response = f.config
		case "/eportal/portal/online_list":
			if f.pendingOnline {
				if f.loginDelayQueries == 0 {
					f.online, f.pendingOnline = true, false
				} else {
					f.loginDelayQueries--
				}
			}
			if f.statusResponse != "" {
				response = f.statusResponse
			} else if f.online {
				response = fmt.Sprintf(`{"result":1,"list":[{"user_account":%q,"online_ip":"10.9.8.7","online_mac":"AABBCCDDEEFF","online_session":"42"}],"total":1}`, f.uid)
			} else {
				response = `{"result":0,"msg":"获取用户在线信息数据为空！"}`
			}
		case "/eportal/portal/login":
			response = f.loginResponse
			if resultCode([]byte(response)) == "1" && !f.loginNoStateChange {
				f.online = f.loginDelayQueries == 0
				f.pendingOnline = !f.online
				f.uid = strings.TrimPrefix(strings.TrimPrefix(r.URL.Query().Get("user_account"), ",0,"), ",1,")
			}
		case "/eportal/portal/logout":
			response = f.logoutResponse
			if resultCode([]byte(response)) == "ok" {
				f.online = false
			}
		case "/eportal/portal/self":
			response = `{"result":"ok","self_auth_url":"http://10.10.244.240:8080/Self/login/eportalLogin?params=fixture-params&timestamp=fixture-timestamp&sign=fixture-sign"}`
		case "/eportal/portal/change_pass":
			response = `{"result":"ok"}`
		case "/eportal/portal/err_code":
			response = `{"result":1,"error_prompt_zh":"test"}`
		default:
			t.Errorf("unexpected portal path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, "%s(%s);", callback, response)
	}))
	t.Cleanup(server.Close)
	p, err := newPortal(server.Client(), "10.9.8.7", terminal, 804)
	if err != nil {
		t.Fatal(err)
	}
	p.api = server.URL + "/eportal/portal/"
	p.stateTimeout = 20 * time.Millisecond
	f.portal = p
	return f
}

func (f portalRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func resultCode(raw []byte) string {
	obj, _ := decodeObject(raw)
	return scalar(obj["result"])
}
