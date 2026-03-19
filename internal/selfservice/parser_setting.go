package selfservice

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var windowUserPattern = regexp.MustCompile(`(?s)\(function\s*\(user\)\s*\{\s*window\.user\s*=\s*user\s*\|\|\s*\{\};\s*\}\)\((\{.*?\})\);\s*</script>`)

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

func extractWindowUserFields(body []byte) map[string]string {
	matches := windowUserPattern.FindSubmatch(body)
	if len(matches) < 2 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(matches[1], &payload); err != nil {
		return nil
	}

	projected := map[string]string{}
	copyProjectedField(projected, payload, "accessGrant")
	copyProjectedField(projected, payload, "bindCmFlag")
	copyProjectedField(projected, payload, "installDate")
	copyProjectedField(projected, payload, "installmentFlag")
	copyProjectedField(projected, payload, "internetDownFlow")
	copyProjectedField(projected, payload, "internetUpFlow")
	copyProjectedField(projected, payload, "leftFlow")
	copyProjectedField(projected, payload, "leftMoney")
	copyProjectedField(projected, payload, "leftTime")
	copyProjectedField(projected, payload, "macAddress")
	copyProjectedField(projected, payload, "multiLogin")
	copyProjectedField(projected, payload, "payStyle")
	copyProjectedField(projected, payload, "stopDate")
	copyProjectedField(projected, payload, "stopReason")
	copyProjectedField(projected, payload, "useFlow")
	copyProjectedField(projected, payload, "useMoney")
	copyProjectedField(projected, payload, "useTime")
	copyProjectedField(projected, payload, "userId")
	copyProjectedField(projected, payload, "userIdNumber")
	copyProjectedField(projected, payload, "userIp")
	copyProjectedField(projected, payload, "userName")
	copyProjectedField(projected, payload, "userRealName")
	copyProjectedField(projected, payload, "vlanId")

	if serviceDefault, ok := payload["serviceDefault"].(map[string]any); ok {
		if value := stringifyField(serviceDefault["defaultName"]); value != "" {
			projected["serviceDefaultName"] = value
		}
	}
	if userGroup, ok := payload["userGroup"].(map[string]any); ok {
		if value := stringifyField(userGroup["userGroupName"]); value != "" {
			projected["userGroupName"] = value
		}
	}
	return projected
}

func copyProjectedField(dst map[string]string, src map[string]any, key string) {
	if value := stringifyField(src[key]); value != "" {
		dst[key] = value
	}
}

func stringifyField(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return ""
	}
}
