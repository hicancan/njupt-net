package zfw

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/hicancan/njupt-net/v3/network"
	"golang.org/x/net/html"
)

type PublicPage struct {
	Page       string `json:"page"`
	Text       string `json:"text"`
	Available  bool   `json:"available"`
	ContentURL string `json:"content_url"`
	HTTPStatus int    `json:"http_status"`
}

func (s *Session) PublicPage(ctx context.Context, kind string) (*PublicPage, error) {
	if kind != "notice" && kind != "help" && kind != "agreement" {
		return nil, fmt.Errorf("public page must be notice, help or agreement")
	}
	if kind == "help" {
		if s.client.Jar == nil {
			return nil, fmt.Errorf("Self help requires an independent CookieJar")
		}
		if !s.hasManagementCookie() {
			if _, _, _, err := s.request(ctx, http.MethodGet, "login", nil); err != nil {
				return nil, fmt.Errorf("initialize Self public-page session: %w", err)
			}
			if !s.hasManagementCookie() {
				return nil, fmt.Errorf("Self login page did not establish its public-page session Cookie")
			}
		}
	}
	data, final, _, err := s.request(ctx, http.MethodGet, "unlogin/"+kind, nil)
	if err != nil {
		return nil, err
	}
	doc, text, err := publicContent(data)
	if err != nil {
		return nil, err
	}
	result := &PublicPage{Page: kind, Text: text, Available: true, ContentURL: publicURL(final), HTTPStatus: http.StatusOK}
	if kind != "help" {
		return result, nil
	}
	frames := []*html.Node{}
	walk(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "iframe" {
			frames = append(frames, n)
		}
	})
	if len(frames) != 1 {
		return nil, fmt.Errorf("Self help page must contain its single content iframe")
	}
	target, err := final.Parse(attr(frames[0], "src"))
	if err != nil || target.Scheme != s.base.Scheme || target.Host != s.base.Host || target.User != nil || target.RawQuery != "" || target.Fragment != "" || !helpContentPath.MatchString(pathWithoutSession(target.Path)) {
		return nil, fmt.Errorf("Self help iframe does not reference its same-origin helpinfo content")
	}
	result.ContentURL, result.Available = publicURL(target), false
	data, final, _, err = s.request(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		var status *network.HTTPStatusError
		if errors.As(err, &status) {
			result.HTTPStatus = status.StatusCode
		} else {
			result.HTTPStatus = 0
		}
		result.Text = ""
		return result, err
	}
	result.ContentURL, result.Text = publicURL(final), ""
	if final.Scheme != target.Scheme || final.Host != target.Host || final.User != nil || final.RawQuery != "" || final.Fragment != "" || pathWithoutSession(final.Path) != pathWithoutSession(target.Path) {
		return result, fmt.Errorf("Self help content navigation did not reach the discovered helpinfo page")
	}
	_, result.Text, err = publicContent(data)
	if err != nil {
		return result, err
	}
	result.Available = result.Text != "暂无使用帮助信息"
	return result, nil
}

func (s *Session) hasManagementCookie() bool {
	for _, cookie := range s.client.Jar.Cookies(s.base) {
		if cookie.Name == "JSESSIONID" && cookie.Value != "" {
			return true
		}
	}
	return false
}

var helpContentPath = regexp.MustCompile(`^/Self/unlogin/helpinfo/[0-9]+$`)

func publicURL(u *url.URL) string {
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: pathWithoutSession(u.Path)}).String()
}

func publicContent(data []byte) (*html.Node, string, error) {
	if !utf8.Valid(data) {
		return nil, "", fmt.Errorf("Self public page returned non-UTF-8 HTML")
	}
	doc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return nil, "", fmt.Errorf("parse Self public page: %w", err)
	}
	body := find(doc, func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "body" })
	if body == nil || nodeText(body) == "" {
		return nil, "", fmt.Errorf("Self public page has no readable body")
	}
	if loginForm(doc) != nil {
		return nil, "", fmt.Errorf("Self public page unexpectedly returned a login form")
	}
	return doc, nodeText(body), nil
}
