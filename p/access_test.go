package p

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

type portalRoundTrip func(*http.Request) (*http.Response, error)

func TestPortalLoginPreservesNativeReturnCode(t *testing.T) {
	for _, code := range []string{"2", `"2"`, ""} {
		for _, accepted := range []bool{false, true} {
			t.Run(fmt.Sprintf("code=%s/accepted=%t", code, accepted), func(t *testing.T) {
				f := newPortalFixture(t, "pc")
				resultCode, outcome, observations := 0, Rejected, 0
				if accepted {
					resultCode, outcome, observations = 1, Accepted, 1
				}
				f.loginResponse = fmt.Sprintf(`{"result":%d,"msg":"AC999"}`, resultCode)
				if code != "" {
					f.loginResponse = strings.TrimSuffix(f.loginResponse, "}") + `,"ret_code":` + code + "}"
				}
				result, err := f.portal.Login(context.Background(), "student", "secret", "campus")
				if (err == nil) != accepted || result == nil || result.Outcome != outcome || result.Verified != accepted || result.Message != "AC999" {
					t.Fatalf("return code changed native acceptance or message: result=%+v error=%v", result, err)
				}
				if string(result.RetCode) != code || len(f.queries["/eportal/portal/login"]) != 1 || len(f.queries["/eportal/portal/online_list"]) != observations || len(f.requests) != 1+observations {
					t.Fatalf("return code was changed or caused an additional request: result=%+v requests=%v", result, f.requests)
				}
				data, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				var encoded map[string]json.RawMessage
				if err := json.Unmarshal(data, &encoded); err != nil {
					t.Fatal(err)
				}
				value, exists := encoded["ret_code"]
				if exists != (code != "") || string(value) != code {
					t.Fatalf("JSON changed return code type or absence: %s", data)
				}
			})
		}
	}
}

func TestOperationResultOwnsNativeReturnCode(t *testing.T) {
	obj, err := decodeObject([]byte(`{"result":0,"ret_code":2,"msg":"AC999"}`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := operationResult(obj, "login")
	if err == nil || result == nil {
		t.Fatalf("expected rejected login result: result=%+v error=%v", result, err)
	}
	obj["ret_code"][0] = '9'
	if string(result.RetCode) != "2" {
		t.Fatal("operation result shares mutable return-code storage with the response")
	}
}

func TestPortalErrorInfoIsTyped(t *testing.T) {
	f := newPortalFixture(t, "pc")
	f.config = "configuration must not be fetched"
	result, err := f.portal.ErrorInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != Accepted || result.PromptChinese != "test" {
		t.Fatalf("error prompt was lost: %+v", result)
	}
	want := url.Values{"callback": {"dr1"}, "wlan_user_ip": {"10.9.8.7"}, "wlan_user_ipv6": {""}, "wlan_user_mac": {"000000000000"}}
	if len(f.requests) != 1 || !reflect.DeepEqual(f.queries["/eportal/portal/err_code"][0], want) {
		t.Fatalf("error information depends on browser configuration: requests=%v query=%v", f.requests, f.queries["/eportal/portal/err_code"])
	}
}

func TestPortalNativeLoginExactWireAndVerification(t *testing.T) {
	for terminalIndex, terminal := range []string{"pc", "mobile", "hipad", "vipad"} {
		for operator, suffix := range map[string]string{"campus": "", "njxy": "@njxy", "cmcc": "@cmcc"} {
			t.Run(terminal+"/"+operator, func(t *testing.T) {
				f := newPortalFixture(t, terminal)
				f.config = "configuration must not be fetched"
				account, password := "student", "space &+=?#中文"
				raw, err := f.portal.Login(context.Background(), account, password, operator)
				if err != nil {
					t.Fatal(err)
				}
				q := f.queries["/eportal/portal/login"][0]
				prefix := ",0,"
				if terminal == "mobile" {
					prefix = ",1,"
				}
				want := url.Values{
					"callback": {"dr1"}, "enable_r3": {"0"}, "login_method": {"1"},
					"terminal_type": {strconv.Itoa(terminalIndex + 1)},
					"user_account":  {prefix + account + suffix}, "user_password": {password},
					"wlan_user_ip": {"10.9.8.7"}, "wlan_user_ipv6": {""},
				}
				if !reflect.DeepEqual(q, want) {
					t.Fatalf("native login wire differs: got %v want %v", q, want)
				}
				if !raw.Verified || raw.Outcome != Accepted || raw.Status.Terminal.Account != account+suffix || raw.Status.Terminal.Session != "42" {
					t.Fatal("login did not produce verified result")
				}
				if len(f.queries["/eportal/portal/login"]) != 1 || len(f.queries["/eportal/portal/online_list"]) != 1 || len(f.requests) != 2 || f.requests[0] != "/eportal/portal/login" {
					t.Fatal("login must submit once and verify status")
				}
			})
		}
	}
}

func TestPortalNativeLoginIgnoresBrowserCredentialEncoding(t *testing.T) {
	f := newPortalFixture(t, "hipad")
	f.config = strings.ReplaceAll(portalTestConfig, `"account_prefix":"1"`, `"account_prefix":"0"`)
	f.config = strings.ReplaceAll(f.config, `"ipad_terminal_identity":"0"`, `"ipad_terminal_identity":"1"`)
	f.config = strings.ReplaceAll(f.config, `"no_filter_accandpwd":"0"`, `"no_filter_accandpwd":"1"`)
	if _, err := f.portal.Configure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.portal.Login(context.Background(), "student", "space &中文", "njxy"); err != nil {
		t.Fatal(err)
	}
	q := f.queries["/eportal/portal/login"][0]
	if len(q) != 8 || q.Get("user_account") != ",0,student@njxy" || q.Get("user_password") != "space &中文" {
		t.Fatalf("native login reused browser presentation switches: %v", q)
	}
	if len(f.queries["/eportal/portal/page/loadConfig"]) != 1 {
		t.Fatal("login reloaded browser configuration")
	}
}

func TestPortalAlreadyOnlinePreservesNativeAuthenticationRejection(t *testing.T) {
	f := newPortalFixture(t, "pc")
	f.online = true
	f.uid = "someone-else"
	f.loginResponse = `{"result":0,"msg":"AC999","ret_code":2}`
	result, err := f.portal.Login(context.Background(), "student", "wrong", "campus")
	if err == nil || result == nil || result.Outcome != Rejected || result.Verified || result.Message != "AC999" || string(result.RetCode) != "2" || result.Status != nil {
		t.Fatalf("existing session changed the native rejection: result=%+v error=%v", result, err)
	}
	if len(f.requests) != 1 || f.requests[0] != "/eportal/portal/login" {
		t.Fatalf("authentication was replaced or surrounded by a status query: %v", f.requests)
	}
}

func TestPortalLoginWaitsForAccountingWithoutResubmission(t *testing.T) {
	f := newPortalFixture(t, "pc")
	f.loginDelayQueries = 1
	f.portal.stateTimeout = 2 * time.Second
	raw, err := f.portal.Login(context.Background(), "student", "secret", "campus")
	if err != nil {
		t.Fatal(err)
	}
	if !raw.Verified || len(f.queries["/eportal/portal/login"]) != 1 || len(f.queries["/eportal/portal/online_list"]) != 2 {
		t.Fatal("asynchronous accounting must be observed without another login")
	}
}

func TestPortalLoginWaitsForTargetIdentityWithoutResubmission(t *testing.T) {
	for _, test := range []struct {
		name, account, source string
		persistent            bool
	}{
		{name: "previous account", account: "previous@cmcc", source: "10.9.8.7"},
		{name: "other source", account: "student@cmcc", source: "10.9.8.8"},
		{name: "persistent other account", account: "previous@cmcc", source: "10.9.8.7", persistent: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newPortalFixture(t, "pc")
			f.portal.stateTimeout = 2 * time.Second
			if test.persistent {
				f.portal.stateTimeout = 225 * time.Millisecond
			}
			underlying := f.portal.client.Transport
			statusReads := 0
			f.portal.client.Transport = portalRoundTrip(func(r *http.Request) (*http.Response, error) {
				response, err := underlying.RoundTrip(r)
				if err != nil || !strings.HasSuffix(r.URL.Path, "/online_list") {
					return response, err
				}
				statusReads++
				if statusReads == 1 || test.persistent {
					response.Body.Close()
					body := fmt.Sprintf(`%s({"result":1,"list":[{"user_account":%q,"online_ip":%q,"online_mac":"AABBCCDDEEFF","online_session":"previous-session"}],"total":1});`, r.URL.Query().Get("callback"), test.account, test.source)
					response.Body = io.NopCloser(strings.NewReader(body))
					response.ContentLength = int64(len(body))
				}
				return response, nil
			})
			result, err := f.portal.Login(context.Background(), "student", "secret", "cmcc")
			if len(f.queries["/eportal/portal/login"]) != 1 || statusReads < 2 {
				t.Fatalf("target identity was not observed without resubmission: status reads=%d login requests=%d", statusReads, len(f.queries["/eportal/portal/login"]))
			}
			if result == nil || result.Outcome != Accepted || result.Status == nil || result.Status.Source != "10.9.8.7" {
				t.Fatalf("accepted result and last source status were lost: result=%+v error=%v", result, err)
			}
			if test.persistent {
				if !errors.Is(err, context.DeadlineExceeded) || result.Verified || !result.Status.Online || result.Status.Terminal.Account != test.account || result.Status.Terminal.Session != "previous-session" {
					t.Fatalf("wrong identity did not remain accepted and unverified at the deadline: result=%+v error=%v", result, err)
				}
			} else if err != nil || !result.Verified || result.Status.Terminal.Account != "student@cmcc" || result.Status.Terminal.IP != "10.9.8.7" {
				t.Fatalf("target identity did not converge: result=%+v error=%v", result, err)
			}
		})
	}
}

func TestPortalStatusUsesMatchingSessionNotResultFlag(t *testing.T) {
	f := newPortalFixture(t, "pc")
	f.statusResponse = `{"result":1,"list":[{"user_account":"other","online_ip":"10.9.8.8","online_mac":"001122334455"}],"total":1}`
	raw, err := f.portal.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if raw.Online {
		t.Fatal("an online session on another source was mistaken for this terminal")
	}
	for _, response := range []string{
		`{"result":0,"msg":"Radius unavailable"}`,
		`{"result":0,"msg":"获取用户在线信息数据为空！","list":[],"total":0}`,
		`{"result":1,"uid":"legacy","ss4":"001122334455"}`,
		`{"result":1,"list":[],"total":1}`,
		`{"result":1,"list":[{"user_account":"a","online_ip":"10.9.8.7","online_mac":"001122334455"},{"user_account":"b","online_ip":"10.9.8.7","online_mac":"001122334455"}],"total":2}`,
	} {
		f.statusResponse = response
		if _, err = f.portal.Status(context.Background()); err == nil {
			t.Fatal("accepted a missing, inconsistent or ambiguous session list")
		}
	}
}

func TestPortalLoginRejectsUnprovenSuccess(t *testing.T) {
	for _, response := range []string{`{"result":0,"msg":"invalid password"}`, `{"result":2}`, `{"result":1}`} {
		t.Run(response, func(t *testing.T) {
			f := newPortalFixture(t, "pc")
			f.loginResponse = response
			f.loginNoStateChange = true
			if _, err := f.portal.Login(context.Background(), "student", "secret", "campus"); err == nil {
				t.Fatal("accepted rejected, unknown or unverified login")
			}
			if len(f.queries["/eportal/portal/login"]) != 1 {
				t.Fatal("authentication was repeated")
			}
		})
	}
}

func TestPortalLogoutVerifiesAndUsesDeployedFields(t *testing.T) {
	f := newPortalFixture(t, "pc")
	f.online = true
	f.uid = "student"
	raw, err := f.portal.Logout(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	q := f.queries["/eportal/portal/logout"][0]
	if q.Get("user_account") != "drcom" || q.Get("user_password") != "123" || q.Get("ac_logout") != "1" || q.Get("register_mode") != "1" || q.Get("wlan_user_ip") != "10.9.8.7" {
		t.Fatalf("incorrect logout contract: %v", q)
	}
	if raw.Status.Online {
		t.Fatal("logout was not verified offline")
	}
	if _, err := f.portal.Logout(context.Background()); err == nil {
		t.Fatal("offline terminal should not trigger another logout")
	}
	if len(f.queries["/eportal/portal/logout"]) != 1 {
		t.Fatal("logout was repeated")
	}
}

func TestPortalOfflineLogoutDoesNotFetchConfiguration(t *testing.T) {
	f := newPortalFixture(t, "pc")
	f.config = "configuration must not be fetched"
	result, err := f.portal.Logout(context.Background())
	if err == nil || !strings.Contains(err.Error(), "already offline") || result == nil || result.Outcome != NotSubmitted || result.Status == nil || result.Status.Online {
		t.Fatalf("offline logout did not preserve the unsubmitted state: result=%+v error=%v", result, err)
	}
	if len(f.requests) != 1 || f.requests[0] != "/eportal/portal/online_list" {
		t.Fatalf("offline logout fetched unrelated configuration or submitted logout: %v", f.requests)
	}
}

func TestPortalUnknownSubmissionDoesNotObserveOrRepeat(t *testing.T) {
	for _, operation := range []string{"login", "logout"} {
		for _, reply := range []string{"lost", "invalid"} {
			t.Run(operation+"/"+reply, func(t *testing.T) {
				f := newPortalFixture(t, "pc")
				if operation == "logout" {
					f.online, f.uid = true, "student"
				}
				underlying := f.portal.client.Transport
				attempts := 0
				cause := errors.New("response lost")
				f.portal.client.Transport = portalRoundTrip(func(r *http.Request) (*http.Response, error) {
					response, err := underlying.RoundTrip(r)
					if err != nil || !strings.HasSuffix(r.URL.Path, "/"+operation) {
						return response, err
					}
					attempts++
					response.Body.Close()
					if reply == "lost" {
						return nil, cause
					}
					body := `wrongCallback({"result":1});`
					response.Body = io.NopCloser(strings.NewReader(body))
					response.ContentLength = int64(len(body))
					return response, nil
				})
				var result *OperationResult
				var err error
				if operation == "login" {
					result, err = f.portal.Login(context.Background(), "student", "do-not-leak?&=", "campus")
				} else {
					result, err = f.portal.Logout(context.Background())
				}
				if err == nil || !strings.Contains(err.Error(), "outcome unknown") || result == nil || result.Outcome != Unknown || result.Verified || result.Status != nil {
					t.Fatalf("unreadable submission did not remain unknown: result=%+v error=%v", result, err)
				}
				if reply == "lost" && !errors.Is(err, cause) {
					t.Fatalf("submission error cause was lost: %v", err)
				}
				if strings.Contains(err.Error(), "do-not-leak") || strings.Contains(err.Error(), "user_password") {
					t.Fatal("error contains secret query")
				}
				observations := 0
				if operation == "logout" {
					observations = 1
				}
				if attempts != 1 || len(f.queries["/eportal/portal/online_list"]) != observations {
					t.Fatalf("unknown submission triggered observation or resubmission: attempts=%d requests=%v", attempts, f.requests)
				}
				state, err := f.portal.Status(context.Background())
				if err != nil || state.Online != (operation == "login") || result.Outcome != Unknown || result.Verified || result.Status != nil {
					t.Fatalf("explicit observation changed the original outcome: state=%+v result=%+v error=%v", state, result, err)
				}
			})
		}
	}
}
