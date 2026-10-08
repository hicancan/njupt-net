package zfw

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hicancan/njupt-net/v4/network"
)

const testBridgeURL = "http://10.10.244.240:8080/Self/login/eportalLogin?params=fixture-params&timestamp=fixture-timestamp&sign=fixture-sign"

// Keep the real URL, origin, Cookie scoping and redirect policy while routing
// fixture sockets locally. No campus service is contacted by these tests.
func bridgeTestClient(t *testing.T, handler http.HandlerFunc) *Session {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	link, err := network.NewLink("127.0.0.1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(link.Close)
	s := New(link)
	dialer := &net.Dialer{Timeout: time.Second}
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	s.client.Transport = transport
	t.Cleanup(transport.CloseIdleConnections)
	return s
}

func TestBridgeOwnsItsActualOriginAndVerifiedIdentity(t *testing.T) {
	bridgeCalls, businessCalls, logoutCalls := 0, 0, 0
	s := bridgeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "10.10.244.240:8080" {
			t.Errorf("bridge management session changed its origin: %s", r.Host)
		}
		switch pathWithoutSession(r.URL.Path) {
		case "/Self/login/eportalLogin":
			bridgeCalls++
			if r.Method != http.MethodGet || len(r.URL.Query()) != 3 || r.URL.Query().Get("params") != "fixture-params" {
				t.Error("bridge did not preserve the selected signed query")
			}
			if cookie, err := r.Cookie("JSESSIONID"); err == nil && cookie.Value == "hostname-session" {
				t.Error("bridge copied a Cookie from the hostname to the IP origin")
			}
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "bridge-session", Path: "/Self"})
			http.Redirect(w, r, "/Self/dashboard;jsessionid=bridge-session", http.StatusFound)
		case "/Self/dashboard":
			cookie, err := r.Cookie("JSESSIONID")
			if err != nil || cookie.Value != "bridge-session" {
				t.Errorf("bridge navigation lost its Cookie: %v", err)
			}
			fmt.Fprint(w, selfTestPage("", "fixture-account"))
		case "/Self/dashboard/getOnlineList":
			businessCalls++
			fmt.Fprint(w, "[]")
		case "/Self/login/logout":
			logoutCalls++
			http.Redirect(w, r, "/Self/login", http.StatusFound)
		case "/Self/login":
			fmt.Fprint(w, selfTestLoginPage())
		default:
			t.Errorf("bridge attempted an unrelated or password endpoint: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	})
	s.client.Jar.SetCookies(s.base, []*http.Cookie{{Name: "JSESSIONID", Value: "hostname-session", Path: "/Self"}})
	if err := s.LoginBridge(context.Background(), "fixture-account", testBridgeURL); err != nil {
		t.Fatal(err)
	}
	if s.identity != "fixture-account" || !s.authenticated || s.base.Host != "10.10.244.240:8080" {
		t.Fatal("bridge did not retain its verified account and actual origin")
	}
	if _, err := s.Online(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if bridgeCalls != 1 || businessCalls != 1 || logoutCalls != 1 || s.authenticated {
		t.Fatalf("bridge=%d business=%d logout=%d authenticated=%t", bridgeCalls, businessCalls, logoutCalls, s.authenticated)
	}
}

func TestBridgeClosesAnUnexpectedAuthenticatedIdentity(t *testing.T) {
	logoutCalls := 0
	s := bridgeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Self/login/eportalLogin":
			http.Redirect(w, r, "/Self/dashboard", http.StatusFound)
		case "/Self/dashboard":
			fmt.Fprint(w, selfTestPage("", "another-account"))
		case "/Self/login/logout":
			logoutCalls++
			http.Redirect(w, r, "/Self/login", http.StatusFound)
		case "/Self/login":
			fmt.Fprint(w, selfTestLoginPage())
		default:
			t.Error("rejected bridge attempted a password fallback")
		}
	})
	if err := s.LoginBridge(context.Background(), "fixture-account", testBridgeURL); err == nil || !strings.Contains(err.Error(), "different account") {
		t.Fatalf("bridge accepted another account: %v", err)
	}
	if logoutCalls != 1 || s.authenticated || s.identity != "" {
		t.Fatalf("rejected bridge was not closed: logout=%d authenticated=%t", logoutCalls, s.authenticated)
	}
}

func TestBridgeRejectsCrossOriginRedirectWithoutPasswordFallback(t *testing.T) {
	calls := 0
	s := bridgeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Redirect(w, r, "http://zfw.njupt.edu.cn:8080/Self/dashboard", http.StatusFound)
	})
	err := s.LoginBridge(context.Background(), "fixture-account", testBridgeURL)
	if err == nil || calls != 1 || s.authenticated || strings.Contains(err.Error(), "fixture-sign") || strings.Contains(err.Error(), "fixture-params") {
		t.Fatalf("bridge redirected or exposed its signed query: calls=%d error=%v", calls, err)
	}
}

func TestBridgeDoesNotReplayItsSignedEntryOrNavigateToAnotherAction(t *testing.T) {
	for _, target := range []string{
		"/Self/login/eportalLogin?params=new&timestamp=new&sign=new",
		"/Self/login/eportalLogin;jsessionid=changed?params=new&timestamp=new&sign=new",
		"/Self/login/logout", "/Self/dashboard/tooffline?sessionid=fixture",
	} {
		t.Run(target, func(t *testing.T) {
			calls := 0
			s := bridgeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				http.Redirect(w, r, target, http.StatusFound)
			})
			if err := s.LoginBridge(context.Background(), "fixture-account", testBridgeURL); err == nil || calls != 1 || s.authenticated {
				t.Fatalf("signed navigation repeated or called another action: calls=%d error=%v", calls, err)
			}
		})
	}
}

func TestBridgeClosesItsCookieSessionAfterAnUnreadableLoginReply(t *testing.T) {
	logouts := 0
	s := bridgeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Self/login/eportalLogin":
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "possibly-authenticated", Path: "/Self"})
			w.Header().Set("Content-Length", "1000")
			fmt.Fprint(w, "incomplete reply")
		case "/Self/login/logout":
			logouts++
			cookie, err := r.Cookie("JSESSIONID")
			if err != nil || cookie.Value != "possibly-authenticated" {
				t.Errorf("unknown login cleanup lost its own Cookie: %v", err)
			}
			http.Redirect(w, r, "/Self/login", http.StatusFound)
		case "/Self/login":
			fmt.Fprint(w, selfTestLoginPage())
		default:
			t.Error("unreadable bridge reply triggered another login")
		}
	})
	err := s.LoginBridge(context.Background(), "fixture-account", testBridgeURL)
	if err == nil || logouts != 1 || s.authenticated || s.loginSubmitted {
		t.Fatalf("unknown login was not closed: logouts=%d authenticated=%t submitted=%t error=%v", logouts, s.authenticated, s.loginSubmitted, err)
	}
}

func TestBridgeURLContractRejectsUnconfirmedEntries(t *testing.T) {
	for _, raw := range []string{
		strings.Replace(testBridgeURL, "http:", "https:", 1),
		strings.Replace(testBridgeURL, "10.10.244.240", "example.com", 1),
		strings.Replace(testBridgeURL, ":8080", ":80", 1),
		strings.Replace(testBridgeURL, "/eportalLogin", "/verify", 1),
		strings.Replace(testBridgeURL, "http://", "http://user:password@", 1),
		testBridgeURL + "&sign=duplicate",
		testBridgeURL + "&extra=value",
		testBridgeURL + "#fragment",
		strings.Replace(testBridgeURL, "params=fixture-params", "params=", 1),
	} {
		if err := ValidateBridgeURL(raw); err == nil {
			t.Fatal("unconfirmed bridge entry was accepted")
		}
	}
	if err := ValidateBridgeURL(testBridgeURL); err != nil {
		t.Fatal(err)
	}
}
