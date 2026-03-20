package portal

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

func parseJSONPPayload(raw string) (map[string]any, error) {
	body := strings.TrimSpace(raw)
	prefix := jsonpCallback + "("
	if !strings.HasPrefix(body, prefix) {
		return nil, fmt.Errorf("invalid jsonp prefix")
	}
	body = strings.TrimPrefix(body, prefix)
	body = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(body, ");"), ")"))
	if body == "" {
		return nil, fmt.Errorf("empty jsonp payload")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func isPortal802AlreadyOnline(retCode, msg string) bool {
	return strings.TrimSpace(retCode) == "2" && strings.EqualFold(strings.TrimSpace(msg), "AC999")
}

func logout801Succeeded(body string) bool {
	return strings.Contains(strings.ToLower(body), "logout succeed")
}

func login801LooksLikeGenericShell(body string) bool {
	normalized := strings.ToLower(body)
	return strings.Contains(normalized, `<div id=app`) &&
		strings.Contains(normalized, "/eportal/public/static/js/app") &&
		strings.Contains(normalized, "<title>eportal</title>")
}

func parseLogin801Response(raw string, endpoint string, adminConsoleDetected bool) (*kernel.Portal801LoginResponse, error) {
	var payload struct {
		Code int         `json:"code"`
		Msg  string      `json:"msg"`
		Data interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return nil, err
	}

	result := &kernel.Portal801LoginResponse{
		Endpoint:             endpoint,
		Code:                 payload.Code,
		Msg:                  strings.TrimSpace(payload.Msg),
		AdminConsoleDetected: adminConsoleDetected,
		RawPayload:           raw,
	}

	if data, ok := payload.Data.(map[string]any); ok {
		if token := kernel.ToString(data["token"]); token != "" {
			result.TokenPresent = true
		}
		result.ChangePass = truthy(data["changepass"])
	}

	return result, nil
}

func truthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		normalized := strings.TrimSpace(strings.ToLower(typed))
		return normalized == "1" || normalized == "true" || normalized == "yes"
	case float64:
		return typed != 0
	default:
		rendered := strings.TrimSpace(fmt.Sprint(value))
		return rendered != "" && rendered != "0"
	}
}
