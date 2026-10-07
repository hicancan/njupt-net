package zfw

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestSelfPublicPageOmitsScripts(t *testing.T) {
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Self/unlogin/notice" {
			t.Error("public page used an incorrect endpoint")
		}
		fmt.Fprint(w, `<html><body><h1>通知公告</h1><p>账号密码登录</p><script>var secret = "do not output";</script></body></html>`)
	})
	page, err := s.PublicPage(context.Background(), "notice")
	if err != nil || page.Text != "通知公告 账号密码登录" || !page.Available || page.HTTPStatus != http.StatusOK {
		t.Fatalf("public page = %v, %v", page, err)
	}
}

func TestHelpFollowsTheDiscoveredContentID(t *testing.T) {
	for _, content := range []string{"暂无使用帮助信息", "校园网使用步骤"} {
		calls := []string{}
		s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, r.URL.Path)
			if r.URL.Path == "/Self/login" {
				http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "public-session", Path: "/Self"})
				fmt.Fprint(w, `<html><body>登录页</body></html>`)
				return
			}
			if r.URL.Path == "/Self/unlogin/help" {
				fmt.Fprint(w, `<html><body>使用帮助<iframe src="/Self/unlogin/helpinfo/0"></iframe><a href="/Self/unlogin/helpinfo/0">打开帮助</a></body></html>`)
				return
			}
			if r.URL.Path != "/Self/unlogin/helpinfo/0" {
				t.Errorf("help content ID was lost: %s", r.URL.Path)
			}
			fmt.Fprint(w, `<html><body><p>`+content+`</p></body></html>`)
		})
		page, err := s.PublicPage(context.Background(), "help")
		if err != nil || page.Text != content || page.Available != (content != "暂无使用帮助信息") || page.HTTPStatus != http.StatusOK || !strings.HasSuffix(page.ContentURL, "/Self/unlogin/helpinfo/0") || len(calls) != 3 || calls[0] != "/Self/login" {
			t.Fatalf("help=%+v calls=%v error=%v", page, calls, err)
		}
	}
}

func TestHelpRejectsAnUnrelatedIframeBeforeRequest(t *testing.T) {
	for _, src := range []string{"https://example.com/collect", "/Self/service/consumeProtect", "/Self/unlogin/helpinfo/"} {
		calls := 0
		s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.URL.Path == "/Self/login" {
				http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "public-session", Path: "/Self"})
				return
			}
			fmt.Fprintf(w, `<html><body>帮助<iframe src="%s"></iframe></body></html>`, src)
		})
		if _, err := s.PublicPage(context.Background(), "help"); err == nil || calls != 2 {
			t.Fatalf("unrelated iframe was requested: src=%s calls=%d error=%v", src, calls, err)
		}
	}
}

func TestHelpContentHTTPFailurePreservesObservedStatus(t *testing.T) {
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Self/login" {
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "public-session", Path: "/Self"})
			return
		}
		if r.URL.Path == "/Self/unlogin/help" {
			fmt.Fprint(w, `<html><body>帮助<iframe src="/Self/unlogin/helpinfo/0"></iframe></body></html>`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	page, err := s.PublicPage(context.Background(), "help")
	if err == nil || page == nil || page.Available || page.HTTPStatus != http.StatusNotFound || page.Text != "" {
		t.Fatalf("content failure became successful help: page=%+v error=%v", page, err)
	}
}

func TestHelpRejectsAContentRedirectToAnotherPage(t *testing.T) {
	for _, destination := range []string{"/Self/unlogin/notice", "/Self/unlogin/helpinfo/1"} {
		t.Run(destination, func(t *testing.T) {
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/Self/login":
					http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "public-session", Path: "/Self"})
				case "/Self/unlogin/help":
					fmt.Fprint(w, `<html><body>帮助<iframe src="/Self/unlogin/helpinfo/0"></iframe></body></html>`)
				case "/Self/unlogin/helpinfo/0":
					http.Redirect(w, r, destination, http.StatusFound)
				default:
					fmt.Fprint(w, `<html><body>无关页面正文</body></html>`)
				}
			})
			page, err := s.PublicPage(context.Background(), "help")
			if err == nil || page == nil || page.Available || page.Text != "" || page.HTTPStatus != http.StatusOK || !strings.HasSuffix(page.ContentURL, destination) {
				t.Fatalf("redirected content became discovered help: page=%+v error=%v", page, err)
			}
		})
	}
}

func TestHelpPreservesAnExistingSessionCookie(t *testing.T) {
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Self/login" {
			t.Error("existing help session was needlessly initialized again")
		}
		cookie, err := r.Cookie("JSESSIONID")
		if err != nil || cookie.Value != "existing-session" {
			t.Errorf("help lost its existing session: %v", err)
		}
		if r.URL.Path == "/Self/unlogin/help" {
			fmt.Fprint(w, `<html><body>帮助<iframe src="/Self/unlogin/helpinfo/0"></iframe></body></html>`)
			return
		}
		fmt.Fprint(w, `<html><body>暂无使用帮助信息</body></html>`)
	})
	s.client.Jar.SetCookies(s.base, []*http.Cookie{{Name: "JSESSIONID", Value: "existing-session", Path: "/Self"}})
	page, err := s.PublicPage(context.Background(), "help")
	if err != nil || page.Available {
		t.Fatalf("existing-session help=%+v error=%v", page, err)
	}
}
