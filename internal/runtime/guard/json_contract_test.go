package guard

import (
	"encoding/json"
	"testing"
)

func TestStatusJSONContract(t *testing.T) {
	status := Status{
		Running:        true,
		Health:         HealthHealthy,
		DesiredProfile: "B",
		ScheduleWindow: "day",
		Binding: BindingStatus{
			Audited: true,
			OK:      true,
			Message: "binding audit ok",
		},
		Connectivity: ConnectivityStatus{
			InitialOK:      true,
			InitialMessage: "internet ok",
			FinalOK:        true,
			FinalMessage:   "internet ok",
			Probe: &ProbeStatus{
				SelectedIP:      "10.163.177.138",
				RoutedIP:        "10.163.177.138",
				SelectionReason: "matches-routed-ip,campus-ip,preferred-interface",
			},
		},
		Portal: PortalStatus{
			Attempted: true,
			OK:        true,
			Message:   "portal login restored connectivity",
			InitialProbe: &ProbeStatus{
				SelectedIP:      "10.163.177.138",
				RoutedIP:        "10.163.177.138",
				SelectionReason: "matches-routed-ip,campus-ip,preferred-interface",
			},
		},
		Cycle: CycleStatus{
			Index:           7,
			RecoveryStep:    "portal-login",
			LastSwitchAt:    "2026-03-19T10:30:00+08:00",
			SwitchTriggered: true,
			SwitchCompleted: true,
		},
		Timing: TimingStatus{
			Timestamp:      "2026-03-19 10:30:03",
			ElapsedSeconds: 0.7,
		},
		Log: LogStatus{
			Path: "D:/code/github/hicancan/njupt-net-cli/dist/guard/logs/guard-20260319-103003.log",
		},
	}
	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	want := `{"running":true,"health":"healthy","desiredProfile":"B","scheduleWindow":"day","binding":{"audited":true,"ok":true,"message":"binding audit ok"},"connectivity":{"initialOk":true,"initialMessage":"internet ok","finalOk":true,"finalMessage":"internet ok","probe":{"selectedIp":"10.163.177.138","routedIp":"10.163.177.138","selectionReason":"matches-routed-ip,campus-ip,preferred-interface"}},"portal":{"attempted":true,"ok":true,"message":"portal login restored connectivity","initialProbe":{"selectedIp":"10.163.177.138","routedIp":"10.163.177.138","selectionReason":"matches-routed-ip,campus-ip,preferred-interface"}},"cycle":{"index":7,"recoveryStep":"portal-login","lastSwitchAt":"2026-03-19T10:30:00+08:00","switchTriggered":true,"switchCompleted":true},"timing":{"timestamp":"2026-03-19 10:30:03","elapsedSeconds":0.7},"log":{"path":"D:/code/github/hicancan/njupt-net-cli/dist/guard/logs/guard-20260319-103003.log"}}`
	if string(payload) != want {
		t.Fatalf("unexpected status json:\n got %s\nwant %s", payload, want)
	}
}

func TestEventJSONContract(t *testing.T) {
	event := NormalizeEvent(Event{
		Timestamp:      "2026-03-19 10:30:03",
		Kind:           EventBindingRepair,
		CycleIndex:     7,
		DesiredProfile: "B",
		ScheduleWindow: "day",
		Message:        "binding moved from W to B",
		Details: BindingRepairEventDetails{
			Action:        "moved",
			BindingOK:     true,
			HolderProfile: "W",
			RecoveryStep:  "binding-repair-then-portal-login",
			TargetProfile: "B",
		},
	})
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	want := `{"timestamp":"2026-03-19 10:30:03","kind":"binding-repair","cycleIndex":7,"desiredProfile":"B","scheduleWindow":"day","message":"binding moved from W to B","details":{"action":"moved","bindingOk":true,"holderProfile":"W","recoveryStep":"binding-repair-then-portal-login","targetProfile":"B"}}`
	if string(payload) != want {
		t.Fatalf("unexpected event json:\n got %s\nwant %s", payload, want)
	}
}

func TestEveryEventKindNormalizesToTypedDetails(t *testing.T) {
	cases := []Event{
		{Kind: EventStartup, Details: map[string]string{"stateDir": "dist/guard"}},
		{Kind: EventScheduleSwitch, Details: map[string]string{"bindingOk": "true", "internetOk": "true", "portalLoginOk": "false", "recoveryStep": "portal-login"}},
		{Kind: EventBindingAudit, Details: map[string]string{"bindingOk": "true", "recoveryStep": "healthy"}},
		{Kind: EventPortalLogin, Details: map[string]string{"internetOk": "true", "portalLoginOk": "false", "recoveryStep": "portal-login"}},
		{Kind: EventBindingRepair, Details: map[string]string{"action": "moved", "bindingOk": "true", "holderProfile": "W", "targetProfile": "B"}},
		{Kind: EventDegraded, Details: map[string]string{"bindingOk": "false", "internetOk": "false", "portalLoginOk": "false", "error": "still offline"}},
		{Kind: EventShutdown, Details: map[string]string{"reason": "test stop"}},
		{Kind: EventFatal, Details: map[string]string{"error": "panic"}},
	}

	for _, tc := range cases {
		event := NormalizeEvent(tc)
		if event.Details == nil {
			t.Fatalf("expected typed details for %s", tc.Kind)
		}
		if _, err := json.Marshal(event); err != nil {
			t.Fatalf("marshal %s: %v", tc.Kind, err)
		}
	}
}
