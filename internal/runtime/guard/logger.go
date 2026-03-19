package guard

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// EventKind is the stable runtime event taxonomy for the Go guard.
type EventKind string

const (
	EventStartup        EventKind = "startup"
	EventScheduleSwitch EventKind = "schedule-switch"
	EventBindingAudit   EventKind = "binding-audit"
	EventPortalLogin    EventKind = "portal-login"
	EventBindingRepair  EventKind = "binding-repair"
	EventDegraded       EventKind = "degraded"
	EventShutdown       EventKind = "shutdown"
	EventFatal          EventKind = "fatal"
)

// Event is the structured runtime record emitted by the Go guard.
type Event struct {
	Timestamp      string    `json:"timestamp"`
	Kind           EventKind `json:"kind"`
	CycleIndex     int       `json:"cycleIndex,omitempty"`
	DesiredProfile string    `json:"desiredProfile,omitempty"`
	ScheduleWindow string    `json:"scheduleWindow,omitempty"`
	Message        string    `json:"message,omitempty"`
	Details        any       `json:"details,omitempty"`
}

// Recorder mirrors typed runtime events to both text logs and JSONL event logs.
type Recorder struct {
	mu       sync.Mutex
	textOut  io.Writer
	eventOut io.Writer
	now      func() time.Time
	location *time.Location
}

// NewRecorder creates a recorder for the paired text/event streams.
func NewRecorder(textOut, eventOut io.Writer, location *time.Location) *Recorder {
	return &Recorder{
		textOut:  textOut,
		eventOut: eventOut,
		now:      time.Now,
		location: location,
	}
}

// Emit writes one structured event to both the human log and the JSONL event stream.
func (r *Recorder) Emit(event Event) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	event = NormalizeEvent(event)
	if strings.TrimSpace(event.Timestamp) == "" {
		now := r.now()
		if r.location != nil {
			now = now.In(r.location)
		}
		event.Timestamp = now.Format("2006-01-02 15:04:05")
	}
	if r.textOut != nil {
		_, _ = fmt.Fprintf(r.textOut, "[%s] %s\n", event.Timestamp, r.humanMessage(event))
	}
	if r.eventOut != nil {
		enc := json.NewEncoder(r.eventOut)
		_ = enc.Encode(event)
	}
}

func (r *Recorder) humanMessage(event Event) string {
	parts := []string{string(event.Kind)}
	if event.CycleIndex > 0 {
		parts = append(parts, fmt.Sprintf("cycle=%d", event.CycleIndex))
	}
	if strings.TrimSpace(event.DesiredProfile) != "" {
		parts = append(parts, "desired="+event.DesiredProfile)
	}
	if strings.TrimSpace(event.ScheduleWindow) != "" {
		parts = append(parts, "window="+event.ScheduleWindow)
	}
	if strings.TrimSpace(event.Message) != "" {
		parts = append(parts, event.Message)
	}
	parts = append(parts, humanDetailPairs(event.Details)...)
	return strings.Join(parts, " ")
}

func humanDetailPairs(details any) []string {
	switch value := details.(type) {
	case nil:
		return nil
	case StartupEventDetails:
		return compactPairs("stateDir", value.StateDir)
	case ScheduleSwitchEventDetails:
		return compactPairs(
			"bindingOk", fmt.Sprintf("%t", value.BindingOK),
			"internetOk", fmt.Sprintf("%t", value.InternetOK),
			"portalLoginOk", fmt.Sprintf("%t", value.PortalLoginOK),
			"recoveryStep", value.RecoveryStep,
		)
	case BindingAuditEventDetails:
		return compactPairs(
			"bindingOk", fmt.Sprintf("%t", value.BindingOK),
			"recoveryStep", value.RecoveryStep,
		)
	case PortalLoginEventDetails:
		return compactPairs(
			"internetOk", fmt.Sprintf("%t", value.InternetOK),
			"portalLoginOk", fmt.Sprintf("%t", value.PortalLoginOK),
			"recoveryStep", value.RecoveryStep,
		)
	case BindingRepairEventDetails:
		return compactPairs(
			"action", value.Action,
			"bindingOk", fmt.Sprintf("%t", value.BindingOK),
			"holderProfile", value.HolderProfile,
			"targetProfile", value.TargetProfile,
			"recoveryStep", value.RecoveryStep,
		)
	case DegradedEventDetails:
		return compactPairs(
			"bindingOk", fmt.Sprintf("%t", value.BindingOK),
			"internetOk", fmt.Sprintf("%t", value.InternetOK),
			"portalLoginOk", fmt.Sprintf("%t", value.PortalLoginOK),
			"recoveryStep", value.RecoveryStep,
			"error", value.Error,
		)
	case ShutdownEventDetails:
		return compactPairs("reason", value.Reason)
	case FatalEventDetails:
		return compactPairs("error", value.Error)
	default:
		return nil
	}
}

func compactPairs(values ...string) []string {
	out := []string{}
	for index := 0; index+1 < len(values); index += 2 {
		if pair := compactPair(values[index], values[index+1]); pair != "" {
			out = append(out, pair)
		}
	}
	return out
}

func compactPair(key, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return fmt.Sprintf("%s=%s", key, value)
}
