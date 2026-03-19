package selfservice

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var businessMessagePattern = regexp.MustCompile(`([\p{Han}A-Za-z0-9@:,_\-]+(?:失败|错误|未绑定|已存在)[^<>\r\n]{0,120})`)

func extractLoginErrorMessage(body []byte) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return "login failed"
	}

	if text := extractText(doc, ".alert-danger", ".alert", "div.error", "span.error", "#error", ".swal2-content", ".swal-content"); text != "" {
		return text
	}

	fallback := ""
	doc.Find("div,span,p,li").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		text := normalizeText(s.Text())
		if looksLikeErrorMessage(text) {
			fallback = text
			return false
		}
		return true
	})
	if fallback != "" {
		return fallback
	}
	return "login failed"
}

func looksLikeErrorMessage(s string) bool {
	lowered := strings.ToLower(strings.TrimSpace(s))
	for _, keyword := range []string{"失败", "错误", "invalid", "error", "fail", "not valid", "登录"} {
		if strings.Contains(lowered, keyword) {
			return true
		}
	}
	return false
}

func extractBusinessMessage(body []byte) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err == nil {
		if text := extractText(doc, ".alert-danger", ".alert", "div.error", "span.error", "#error", ".swal2-content", ".swal-content", ".layui-layer-content"); text != "" {
			return text
		}
		fallback := ""
		doc.Find("div,span,p,li").EachWithBreak(func(_ int, s *goquery.Selection) bool {
			text := normalizeText(s.Text())
			if looksLikeBindingBusinessMessage(text) {
				fallback = text
				return false
			}
			return true
		})
		if fallback != "" {
			return fallback
		}
	}

	raw := normalizeText(string(body))
	if matches := businessMessagePattern.FindStringSubmatch(raw); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func looksLikeBindingBusinessMessage(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return false
	}
	for _, keyword := range []string{"绑定失败", "未绑定", "已存在", "运营商账号", "失败"} {
		if strings.Contains(trimmed, keyword) {
			return true
		}
	}
	return looksLikeErrorMessage(trimmed)
}
