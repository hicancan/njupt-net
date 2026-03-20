package kernel

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ToString normalizes loosely typed JSON and HTML parser values into stable strings.
func ToString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case json.Number:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

// CaptureRaw converts a transport response into the stable diagnostic capture model.
func CaptureRaw(resp *SessionResponse) *RawCapture {
	if resp == nil {
		return nil
	}
	return &RawCapture{
		Status:   resp.StatusCode,
		Headers:  resp.Headers,
		Body:     string(resp.Body),
		FinalURL: resp.FinalURL,
	}
}
