package network

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRequestDoesNotExposeCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	_, _, _, err := Request(context.Background(), server.Client(), http.MethodGet, server.URL+"/login", url.Values{"user_password": {"secret-value"}})
	if err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestRequestRejectsNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	if _, _, _, err := Request(context.Background(), server.Client(), http.MethodGet, server.URL, nil); err == nil {
		t.Fatal("204 was accepted as a valid protocol response")
	}
}

func TestStatusErrorPreservesCodeWithoutQueryOrSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }))
	defer server.Close()
	_, _, _, err := Request(context.Background(), server.Client(), http.MethodGet, server.URL+"/help;jsessionid=private-session", url.Values{"password": {"private-password"}})
	var status *HTTPStatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusNotFound || status.Method != http.MethodGet {
		t.Fatalf("HTTP observation lost its status: %v", err)
	}
	if strings.Contains(err.Error(), "private-session") || strings.Contains(err.Error(), "private-password") {
		t.Fatalf("HTTP status included request credentials: %v", err)
	}
}

func TestMalformedRedirectLocationDoesNotExposeItsSignedQuery(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Location", "/Self/%zz?sign=synthetic-sensitive-value")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	_, _, _, err := Request(context.Background(), server.Client(), http.MethodGet, server.URL+"/Self/login", nil)
	if err == nil || calls != 1 || !strings.Contains(err.Error(), "invalid HTTP redirect Location") || strings.Contains(err.Error(), "synthetic-sensitive-value") || strings.Contains(err.Error(), "sign=") {
		t.Fatalf("malformed redirect exposed its target or retried: calls=%d error=%v", calls, err)
	}
}

func TestRedirectPolicyErrorDoesNotExposeItsSignedQuery(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Location", "/Self/dashboard?sign=synthetic-sensitive-value")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return fmt.Errorf("reject this complete URL: %s", req.URL.String())
	}
	_, _, _, err := Request(context.Background(), client, http.MethodGet, server.URL+"/Self/login", nil)
	if err == nil || calls != 1 || !strings.Contains(err.Error(), "HTTP redirect rejected") || strings.Contains(err.Error(), "synthetic-sensitive-value") || strings.Contains(err.Error(), "sign=") {
		t.Fatalf("redirect policy exposed its target or retried: calls=%d error=%v", calls, err)
	}
}

func TestRequestStillPreservesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _, err := Request(ctx, http.DefaultClient, http.MethodGet, "http://127.0.0.1:1/Self/login", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("request error sanitization lost cancellation identity: %v", err)
	}
}

func TestLinkIgnoresProxyAndBindsSource(t *testing.T) {
	var remote string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { remote = r.RemoteAddr; w.Write([]byte("ok")) }))
	defer server.Close()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	network, err := NewLink("127.0.0.1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer network.Close()
	if _, _, _, err := Request(context.Background(), network.Client(), http.MethodGet, server.URL, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(remote, "127.0.0.1:") {
		t.Fatalf("unexpected source: %s", remote)
	}
}

func TestLinkRejectsCrossHostRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/", http.StatusFound)
	}))
	defer server.Close()
	network, _ := NewLink("127.0.0.1", time.Second)
	defer network.Close()
	if _, _, _, err := Request(context.Background(), network.Client(), http.MethodGet, server.URL, nil); err == nil {
		t.Fatal("unexpected destination accepted")
	}
}

func TestLinkDoesNotReplayRedirectedSubmission(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				http.Redirect(w, r, "/submit-again", status)
			}))
			defer server.Close()
			network, _ := NewLink("127.0.0.1", time.Second)
			defer network.Close()
			_, _, _, err := Request(context.Background(), network.Client(), http.MethodPost, server.URL+"/submit", url.Values{"password": {"fixture"}})
			if err == nil || calls != 1 {
				t.Fatalf("submission was replayed: calls=%d, error=%v", calls, err)
			}
		})
	}
}

func TestLinkClientsKeepWebsiteSessionsIndependent(t *testing.T) {
	link, err := NewLink("127.0.0.1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer link.Close()
	portal, management := link.Client(), link.Client()
	endpoint, _ := url.Parse("http://example.test/")
	portal.Jar.SetCookies(endpoint, []*http.Cookie{{Name: "session", Value: "portal"}})
	if cookies := management.Jar.Cookies(endpoint); len(cookies) != 0 {
		t.Fatal("management client inherited portal cookies")
	}
	if portal.Transport != management.Transport {
		t.Fatal("clients lost their common selected link")
	}
	if link.Source() != "127.0.0.1" {
		t.Fatal("link source changed while creating clients")
	}
}
