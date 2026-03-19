package selfservice

import (
	"bytes"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

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
