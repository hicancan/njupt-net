package p

import (
	"bytes"
	"encoding/json"
)

// The portal owns the native JSONP schema. Authentication diagnostics retain
// its result and message; session queries retain only their identity fields.
// Password pages, browser configuration and bridge URLs have no body policy.
func tracePayload(path, callback string) func([]byte) (map[string]any, string) {
	var keys []string
	switch path {
	case "login":
		keys = []string{"result", "msg", "ret_code"}
	case "online_list":
		keys = []string{"result", "msg", "ret_code", "total"}
	case "err_code":
		keys = []string{"result", "msg", "ret_code", "error_code", "error_prompt_zh", "error_prompt_en"}
	default:
		return nil
	}
	return func(body []byte) (map[string]any, string) {
		obj, err := decodeJSONP(body, callback)
		if err != nil {
			return nil, "invalid_jsonp"
		}
		result := make(map[string]any)
		for _, key := range keys {
			copyTraceScalar(result, obj, key)
		}
		if path == "online_list" {
			var rows []map[string]json.RawMessage
			if raw, ok := obj["list"]; ok && json.Unmarshal(raw, &rows) == nil {
				list := make([]any, 0, len(rows))
				for _, row := range rows {
					entry := make(map[string]any)
					for _, key := range []string{"user_account", "online_ip", "online_mac", "online_session"} {
						copyTraceScalar(entry, row, key)
					}
					list = append(list, entry)
				}
				result["list"] = list
			}
		}
		return result, "jsonp"
	}
}

// Native fields are scalars. An unexpected nested object is not a diagnostic
// extension of the schema and must not expose arbitrary response content.
func copyTraceScalar(destination map[string]any, source map[string]json.RawMessage, key string) {
	raw, ok := source[key]
	if !ok {
		return
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return
	}
	switch value.(type) {
	case string, json.Number, bool, nil:
		destination[key] = value
	}
}
