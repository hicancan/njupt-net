package zfw

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func selfTestPage(contents, user string) string {
	return `<html><body>` + contents + `<script>
$.get("/Self/dashboard/refreshaccount", {
    csrftoken: 'refresh-token',
    t: Math.random()
});
(function (user) { window.user = user || {}; })({"userName":"` + user + `","installmentFlag":999999,"userPassword":"server-secret"});
</script></body></html>`
}

func selfTestLoginPage() string {
	return `<html><body><form action="/Self/login/verify;jsessionid=first" method="post">
<input name="foo"><input type="password" name="bar"><input type="hidden" name="checkcode" value="dynamic-code">
<input name="account"><input type="password" name="password"><div id="randomDiv" class="form-group hide"><input name="code"></div>
<button name="submit" type="submit">登录</button></form><script>var md5 = false;</script></body></html>`
}

func selfTestClient(t *testing.T, handler http.HandlerFunc) *Session {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := server.Client()
	client.Jar, _ = cookiejar.New(nil)
	s := (&Session{client: client, identity: "fixture-account", authenticated: true})
	s.base, _ = url.Parse(server.URL + "/Self/")
	return s
}

func TestSelfLoginUsesDynamicFormAndPrefetch(t *testing.T) {
	var calls []string
	ready := false
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path := pathWithoutSession(r.URL.Path)
		calls = append(calls, r.Method+" "+path)
		switch path {
		case "/Self/login/":
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "session", Path: "/Self"})
			fmt.Fprint(w, selfTestLoginPage())
		case "/Self/login/randomCode":
			cookie, err := r.Cookie("JSESSIONID")
			if err != nil || cookie.Value != "session" {
				t.Error("prefetch lost the login page cookie")
			}
			if r.URL.Query().Get("t") == "" {
				t.Error("prefetch lacks its cache nonce")
			}
			ready = true
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("\x89PNG\r\n\x1a\nfixture"))
		case "/Self/login/verify":
			if !ready {
				t.Error("login submitted before randomCode prefetch")
			}
			if r.URL.Path != "/Self/login/verify;jsessionid=first" {
				t.Error("dynamic session-bearing form action was lost")
			}
			r.ParseForm()
			want := url.Values{"foo": {""}, "bar": {""}, "checkcode": {"dynamic-code"}, "account": {"fixture-account"}, "password": {"fixture-password"}, "code": {""}}
			if !reflect.DeepEqual(r.PostForm, want) {
				t.Error("submission differs from successful form controls")
			}
			http.Redirect(w, r, "/Self/dashboard", http.StatusFound)
		case "/Self/dashboard":
			fmt.Fprint(w, selfTestPage("", "fixture-account"))
		default:
			http.NotFound(w, r)
		}
	})
	s.identity, s.authenticated = "", false
	if err := s.Login(context.Background(), "fixture-account", "fixture-password"); err != nil {
		t.Fatal(err)
	}
	if s.identity != "fixture-account" {
		t.Fatal("authenticated identity was not retained")
	}
	want := []string{"GET /Self/login/", "GET /Self/login/randomCode", "POST /Self/login/verify", "GET /Self/dashboard"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestSelfLoginRejectsMismatchedIdentity(t *testing.T) {
	logouts := 0
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch pathWithoutSession(r.URL.Path) {
		case "/Self/login/":
			fmt.Fprint(w, selfTestLoginPage())
		case "/Self/login/randomCode":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("\x89PNG\r\n\x1a\n"))
		case "/Self/login/verify":
			http.Redirect(w, r, "/Self/dashboard", http.StatusFound)
		case "/Self/dashboard":
			fmt.Fprint(w, selfTestPage("", "another-account"))
		case "/Self/login/logout":
			logouts++
			http.Redirect(w, r, "/Self/login/", http.StatusFound)
		}
	})
	s.identity, s.authenticated = "", false
	if err := s.Login(context.Background(), "fixture-account", "fixture-password"); err == nil || !strings.Contains(err.Error(), "different account") {
		t.Fatalf("expected identity failure, got %v", err)
	}
	if s.identity != "" {
		t.Fatal("identity failure retained the authenticated state")
	}
	if logouts != 1 || s.authenticated {
		t.Fatalf("rejected management session was not closed: logouts=%d authenticated=%t", logouts, s.authenticated)
	}
}

func TestLoginDoesNotUseRefreshTokenAsAuthenticationState(t *testing.T) {
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch pathWithoutSession(r.URL.Path) {
		case "/Self/login/":
			fmt.Fprint(w, selfTestLoginPage())
		case "/Self/login/randomCode":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("\x89PNG\r\n\x1a\n"))
		case "/Self/login/verify":
			http.Redirect(w, r, "/Self/dashboard", http.StatusFound)
		case "/Self/dashboard":
			fmt.Fprint(w, `<html><body><script>(function(user){})({"userName":"fixture-account"});</script></body></html>`)
		}
	})
	s.identity, s.authenticated = "", false
	if err := s.Login(context.Background(), "fixture-account", "fixture-password"); err != nil {
		t.Fatal(err)
	}
	if !s.authenticated || s.identity != "fixture-account" {
		t.Fatal("login did not own its authenticated identity")
	}
}

func TestSelfJSONDistinguishesExpiredEmptyAndMalformedResponses(t *testing.T) {
	for _, test := range []struct {
		name, body, want string
		expired          bool
	}{
		{"login", selfTestLoginPage(), "", true},
		{"empty", "", "empty JSON", false},
		{"malformed", "not-json", "invalid JSON", false},
		{"invalid UTF-8", "[\"\xff\"]", "invalid JSON", false},
		{"html", "<html><body>error</body></html>", "HTML instead", false},
		{"null", "null", "did not return an array", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("ajaxCsrfToken") != "" {
					t.Error("request invented window.AJAXCSRFTOKEN")
				}
				if r.URL.Query().Get("t") == "" {
					t.Error("request lacks its cache nonce")
				}
				fmt.Fprint(w, test.body)
			})
			s.identity = "fixture-account"
			_, err := s.Online(context.Background())
			if test.expired {
				if !errors.Is(err, ErrSessionExpired) {
					t.Fatalf("expected expired session, got %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}

func TestSelfLanguageUsesActualValuesAndVerifiesButton(t *testing.T) {
	for _, language := range []string{"English", "zh_cn"} {
		t.Run(language, func(t *testing.T) {
			changes := 0
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/Self/login/changeLanguage" {
					changes++
					if r.URL.Query().Get("language") != language || !strings.HasPrefix(r.URL.Query().Get(""), "t0.") {
						t.Error("language request differs from changeLanguage.js")
					}
					return
				}
				if r.URL.Path != "/Self/login/" {
					t.Errorf("language verification requested a noncanonical login page: %s", r.URL.Path)
				}
				button := "English"
				if language == "English" {
					button = "中文"
				}
				fmt.Fprintf(w, `<html><body><a id="language">%s</a></body></html>`, button)
			})
			if err := s.Language(context.Background(), language); err != nil || changes != 1 {
				t.Fatalf("changes = %d; error = %v", changes, err)
			}
		})
	}
	if err := (&Session{}).Language(context.Background(), "en_US"); err == nil {
		t.Error("invented language alias was accepted")
	}
}

func TestSelfLogoutRequiresLoginPage(t *testing.T) {
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Self/login/logout" {
			http.Redirect(w, r, "/Self/login/", http.StatusFound)
			return
		}
		fmt.Fprint(w, selfTestLoginPage())
	})
	s.identity = "fixture-account"
	if err := s.Logout(context.Background()); err != nil || s.identity != "" {
		t.Fatalf("logout error = %v; retained token = %t", err, s.identity != "")
	}
}

func TestSelfLogoutFollowsOnlyItsNativeReadNavigation(t *testing.T) {
	var calls []string
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/Self/login/logout":
			http.Redirect(w, r, "/Self/", http.StatusFound)
		case "/Self/":
			http.Redirect(w, r, "/Self/dashboard;jsessionid=own-session", http.StatusFound)
		case "/Self/dashboard;jsessionid=own-session":
			http.Redirect(w, r, "/Self/login/", http.StatusFound)
		case "/Self/login/":
			fmt.Fprint(w, selfTestLoginPage())
		default:
			t.Errorf("logout reached another action: %s", r.URL.Path)
		}
	})
	if err := s.Logout(context.Background()); err != nil || s.authenticated {
		t.Fatalf("logout navigation failed: calls=%v error=%v", calls, err)
	}
	want := []string{"/Self/login/logout", "/Self/", "/Self/dashboard;jsessionid=own-session", "/Self/login/"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected logout requests: %v", calls)
	}
}

func TestSelfWriteRedirectToLoginInvalidatesSessionWithoutNavigation(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, destination := range []string{"/Self/login/", "/Self/login;jsessionid=expired"} {
			t.Run(method+destination, func(t *testing.T) {
				calls := 0
				s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					http.Redirect(w, r, destination, http.StatusFound)
				})
				_, _, _, err := s.request(context.Background(), method, "dashboard/tooffline", nil)
				if !errors.Is(err, ErrSessionExpired) || calls != 1 || s.identity != "" || s.authenticated || s.loginSubmitted {
					t.Fatalf("expired write was followed or retained authentication: calls=%d error=%v", calls, err)
				}
			})
		}
	}
}

func TestSelfWriteRedirectCannotExpireSessionThroughAnotherOrigin(t *testing.T) {
	calls := 0
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Redirect(w, r, "http://example.com/Self/login/", http.StatusFound)
	})
	_, _, _, err := s.request(context.Background(), http.MethodGet, "dashboard/tooffline", nil)
	if err == nil || errors.Is(err, ErrSessionExpired) || calls != 1 || s.identity != "fixture-account" || !s.authenticated {
		t.Fatalf("foreign login redirect changed the session: calls=%d error=%v", calls, err)
	}
}

func TestSelfReadRedirectToLoginInvalidatesSessionWithoutNavigation(t *testing.T) {
	calls := 0
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Redirect(w, r, "/Self/login/", http.StatusFound)
	})
	_, err := s.Online(context.Background())
	if !errors.Is(err, ErrSessionExpired) || calls != 1 || s.identity != "" || s.authenticated {
		t.Fatalf("expired read was followed or retained authentication: calls=%d error=%v", calls, err)
	}
}

func TestLanguageAcceptsItsLoginPageRedirectBeforeAuthentication(t *testing.T) {
	changes, reads := 0, 0
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Self/login/changeLanguage":
			changes++
			http.Redirect(w, r, "/Self/login/", http.StatusFound)
		case "/Self/login/":
			reads++
			fmt.Fprint(w, `<html><body><a id="language">中文</a></body></html>`)
		default:
			t.Error("language navigated to another action")
		}
	})
	s.identity, s.authenticated = "", false
	if err := s.Language(context.Background(), "English"); err != nil || changes != 1 || reads != 2 {
		t.Fatalf("language navigation failed: changes=%d reads=%d error=%v", changes, reads, err)
	}
}

func TestSelfRejectsExternalEndpoints(t *testing.T) {
	base, _ := url.Parse("http://zfw.njupt.edu.cn:8080/Self/")
	s := &Session{base: base}
	for _, endpoint := range []string{"http://example.com/Self/login/verify", "http://zfw.njupt.edu.cn:8080/other", "//example.com/Self/"} {
		if _, _, _, err := s.request(context.Background(), http.MethodPost, endpoint, nil); err == nil {
			t.Error("endpoint escaped Self")
		}
	}
}
