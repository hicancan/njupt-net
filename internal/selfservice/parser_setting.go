package selfservice

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func extractInputFields(doc *goquery.Document) map[string]string {
	fields := map[string]string{}
	doc.Find("input[name]").Each(func(_ int, selection *goquery.Selection) {
		name, ok := selection.Attr("name")
		if !ok || name == "" {
			return
		}
		value, _ := selection.Attr("value")
		fields[name] = value
	})
	return fields
}

func sanitizeSensitiveFields(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return fields
	}
	sanitized := make(map[string]string, len(fields))
	for key, value := range fields {
		if isSensitiveFieldName(key) {
			sanitized[key] = ""
			continue
		}
		sanitized[key] = value
	}
	return sanitized
}

func isSensitiveFieldName(name string) bool {
	lowered := strings.ToLower(strings.TrimSpace(name))
	switch lowered {
	case "password", "userpassword", "oldpassword", "newpassword", "confirmpassword", "upass":
		return true
	}
	return strings.Contains(lowered, "password")
}
