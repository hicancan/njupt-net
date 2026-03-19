package portal

import (
	"encoding/json"
	"fmt"
	"strings"
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

func toString(v any) string {
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	default:
		return strings.TrimSpace(fmt.Sprint(val))
	}
}
