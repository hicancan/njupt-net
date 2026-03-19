package httpx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

func TestSessionClientGetAndPostBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get":
			if r.URL.Query().Get("hello") != "world" {
				t.Fatalf("unexpected query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Location", "/next")
			_, _ = io.WriteString(w, "get-ok")
		case "/post":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if r.Form.Get("a") != "1" {
				t.Fatalf("unexpected form: %#v", r.Form)
			}
			_, _ = io.WriteString(w, "post-ok")
		case "/json":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read json body: %v", err)
			}
			if got, want := string(body), `{"hello":"world"}`; got != want {
				t.Fatalf("unexpected json payload: %q want %q", got, want)
			}
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("unexpected content-type: %q", got)
			}
			_, _ = io.WriteString(w, "json-ok")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewSessionClient(Options{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("new session client: %v", err)
	}

	getResp, err := client.Get(context.Background(), "/get", kernel.RequestOptions{Query: map[string]string{"hello": "world"}})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(getResp.Body) != "get-ok" {
		t.Fatalf("unexpected get body: %q", string(getResp.Body))
	}
	if !strings.HasSuffix(getResp.FinalURL, "/next") {
		t.Fatalf("unexpected get finalURL: %q", getResp.FinalURL)
	}

	postResp, err := client.PostForm(context.Background(), "/post", kernel.RequestOptions{Form: map[string]string{"a": "1"}})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if string(postResp.Body) != "post-ok" {
		t.Fatalf("unexpected post body: %q", string(postResp.Body))
	}

	jsonResp, err := client.PostJSON(context.Background(), "/json", kernel.RequestOptions{}, []byte(`{"hello":"world"}`))
	if err != nil {
		t.Fatalf("post json: %v", err)
	}
	if string(jsonResp.Body) != "json-ok" {
		t.Fatalf("unexpected json body: %q", string(jsonResp.Body))
	}
}

func TestBuildURLRejectsMissingBaseURL(t *testing.T) {
	client, err := NewSessionClient(Options{})
	if err != nil {
		t.Fatalf("new session client: %v", err)
	}
	_, err = client.Get(context.Background(), "/relative", kernel.RequestOptions{})
	if err == nil || !strings.Contains(err.Error(), "relative path requires a baseURL") {
		t.Fatalf("expected baseURL error, got %v", err)
	}
}
