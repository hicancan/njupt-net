package guard

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRecorderUsesConfiguredLocationForHumanTimestamp(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	var text bytes.Buffer
	recorder := NewRecorder(&text, nil, location)
	recorder.now = func() time.Time {
		return time.Date(2026, time.March, 19, 13, 14, 47, 0, time.UTC)
	}
	recorder.Emit(Event{
		Kind:    EventStartup,
		Message: "starting Go guard",
	})

	got := text.String()
	if !strings.Contains(got, "[2026-03-19 21:14:47]") {
		t.Fatalf("expected Asia/Shanghai timestamp, got %q", got)
	}
}

func TestRecorderRedactsSensitiveQueryValues(t *testing.T) {
	var text bytes.Buffer
	var events bytes.Buffer
	recorder := NewRecorder(&text, &events, time.UTC)
	recorder.Emit(Event{
		Kind:    EventDegraded,
		Message: `portal failed: https://portal.example/login?user_account=alice&user_password=secret&wlan_user_ip=10.0.0.1`,
		Details: DegradedEventDetails{
			Error: `Get "https://portal.example/login?password=secret&account=alice": timeout`,
		},
	})

	if strings.Contains(text.String(), "secret") || strings.Contains(text.String(), "alice") {
		t.Fatalf("expected redacted human log, got %q", text.String())
	}
	var event Event
	if err := json.Unmarshal(bytes.TrimSpace(events.Bytes()), &event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	if strings.Contains(string(payload), "secret") || strings.Contains(string(payload), "alice") {
		t.Fatalf("expected redacted event payload, got %s", payload)
	}
	if !strings.Contains(string(payload), "redacted") {
		t.Fatalf("expected redaction marker, got %s", payload)
	}
}
