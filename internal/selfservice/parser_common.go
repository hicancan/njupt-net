package selfservice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func looksLikeLoginPage(body []byte) bool {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return false
	}
	if doc.Find("input[name='checkcode']").Length() > 0 {
		return true
	}
	if doc.Find("input[name='account']").Length() > 0 && doc.Find("input[name='password']").Length() > 0 {
		return true
	}
	if action, ok := doc.Find("form").First().Attr("action"); ok && strings.Contains(strings.ToLower(action), "/self/login") {
		return true
	}
	return false
}

func extractInputValue(doc *goquery.Document, name string) string {
	value, _ := doc.Find("input[name='" + name + "']").First().Attr("value")
	return strings.TrimSpace(value)
}

func extractText(doc *goquery.Document, selectors ...string) string {
	for _, selector := range selectors {
		text := normalizeText(doc.Find(selector).First().Text())
		if text != "" {
			return text
		}
	}
	return ""
}

func normalizeText(s string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(val), 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case json.Number:
		return val.String()
	default:
		return strings.TrimSpace(fmt.Sprint(val))
	}
}

func boolFromJSON(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		lowered := strings.ToLower(strings.TrimSpace(val))
		return lowered == "true" || lowered == "1"
	default:
		return false
	}
}

func parseJSON(body []byte, out interface{}) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	return dec.Decode(out)
}

func firstAttrOrText(doc *goquery.Document, selectors ...string) string {
	for _, selector := range selectors {
		sel := doc.Find(selector).First()
		if sel.Length() == 0 {
			continue
		}
		if value, ok := sel.Attr("value"); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		text := normalizeText(sel.Text())
		if text != "" {
			return text
		}
	}
	return ""
}
