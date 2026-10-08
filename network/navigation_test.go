package network

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRequestNeverFollowsSubmissionRedirects(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, status := range []int{301, 302, 303, 307, 308} {
			t.Run(fmt.Sprintf("%s/%d", method, status), func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					http.Redirect(w, r, "/mutate-again?sign=secret", status)
				}))
				defer server.Close()
				_, _, _, err := Request(context.Background(), server.Client(), method, server.URL+"/mutate", url.Values{"password": {"secret"}})
				var rejected *RedirectError
				if !errors.As(err, &rejected) || calls != 1 || strings.Contains(err.Error(), "secret") {
					t.Fatalf("single submission followed a redirect: calls=%d error=%v", calls, err)
				}
				if rejected.Destination().Path != "/mutate-again" {
					t.Fatal("rejected destination identity was lost")
				}
			})
		}
	}
}

func TestNavigateSubmitsOnceThenReadsConfirmedTargets(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, status := range []int{301, 302, 303} {
			t.Run(fmt.Sprintf("%s/%d", method, status), func(t *testing.T) {
				submissions, reads := 0, 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/submit":
						submissions++
						if r.Method != method {
							t.Errorf("initial method=%s", r.Method)
						}
						http.SetCookie(w, &http.Cookie{Name: "session", Value: "own-session"})
						http.Redirect(w, r, "/result", status)
					case "/result":
						reads++
						body, _ := io.ReadAll(r.Body)
						cookie, err := r.Cookie("session")
						if r.Method != http.MethodGet || len(body) != 0 || r.URL.RawQuery != "" || err != nil || cookie.Value != "own-session" {
							t.Error("navigation replayed submission values or lost its session")
						}
						fmt.Fprint(w, "saved")
					default:
						t.Fatal("unrelated destination reached")
					}
				}))
				defer server.Close()
				client := directTestLink(t).Client()
				data, final, _, err := Navigate(context.Background(), client, method, server.URL+"/submit", url.Values{"secret": {"value"}}, func(u *url.URL) bool { return u.Path == "/result" })
				if err != nil || submissions != 1 || reads != 1 || string(data) != "saved" || final.Path != "/result" {
					t.Fatalf("submissions=%d reads=%d data=%q final=%v error=%v", submissions, reads, data, final, err)
				}
			})
		}
	}
}

func TestNavigateRejectsReplayAndUnconfirmedTargets(t *testing.T) {
	for _, test := range []struct {
		name, target string
		status       int
	}{
		{"307", "/read", 307}, {"308", "/read", 308},
		{"same entry", "/submit?sign=new-secret", 302},
		{"another action", "/mutate-again", 303},
		{"origin", "http://example.com/read", 302},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				http.Redirect(w, r, test.target, test.status)
			}))
			defer server.Close()
			_, _, _, err := Navigate(context.Background(), server.Client(), http.MethodPost, server.URL+"/submit", nil, func(u *url.URL) bool { return u.Path == "/read" || u.Path == "/submit" })
			if err == nil || calls != 1 {
				t.Fatalf("unconfirmed navigation followed: calls=%d error=%v", calls, err)
			}
		})
	}
}

type navigationTransport struct{ reads []bool }

func (t *navigationTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	read, _ := r.Context().Value(readRequestKey{}).(bool)
	t.reads = append(t.reads, read)
	response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("done")), Request: r}
	if r.URL.Path == "/signed-entry" {
		response.StatusCode = http.StatusFound
		response.Header.Set("Location", "/dashboard")
	}
	return response, nil
}

func TestNavigateKeepsSignedEntrySingleUseAndSubsequentReadsReusable(t *testing.T) {
	transport := &navigationTransport{}
	client := &http.Client{Transport: transport}
	_, _, _, err := Navigate(context.Background(), client, http.MethodGet, "http://example.test/signed-entry?sign=secret", nil, func(u *url.URL) bool { return u.Path == "/dashboard" })
	if err != nil || len(transport.reads) != 2 || transport.reads[0] || !transport.reads[1] {
		t.Fatalf("transport selection=%v error=%v", transport.reads, err)
	}
	if client.CheckRedirect != nil {
		t.Fatal("operation changed the shared client policy")
	}
}

func TestReadNeverFollowsUnconfirmedGETRedirects(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				http.Redirect(w, r, "/write-by-get?sign=secret", status)
			}))
			defer server.Close()
			_, _, _, err := Read(context.Background(), server.Client(), server.URL+"/read", nil)
			var rejected *RedirectError
			if !errors.As(err, &rejected) || calls != 1 || strings.Contains(err.Error(), "secret") {
				t.Fatalf("read followed an unconfirmed GET action: calls=%d error=%v", calls, err)
			}
			if rejected.Destination().Path != "/write-by-get" {
				t.Fatal("rejected destination identity was lost")
			}
		})
	}
}

func TestRequestRetainsLastResponsePolicy(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/read" {
			http.Redirect(w, r, "/result", http.StatusFound)
			return
		}
		fmt.Fprint(w, "read")
	}))
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	_, _, _, err := Request(context.Background(), client, http.MethodGet, server.URL+"/read", nil)
	var status *HTTPStatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusFound || calls != 1 {
		t.Fatalf("last-response observation changed: calls=%d error=%v", calls, err)
	}
}
