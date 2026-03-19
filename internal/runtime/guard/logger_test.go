package guard

import (
	"bytes"
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
