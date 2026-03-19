package guard

import "testing"

func TestNormalizeEventUpgradesLegacyDegradedDetails(t *testing.T) {
	event := NormalizeEvent(Event{
		Kind: EventDegraded,
		Details: map[string]string{
			"bindingOk":     "false",
			"internetOk":    "false",
			"portalLoginOk": "false",
			"recoveryStep":  "binding-repair-then-portal-login",
			"error":         "still offline",
			"noise":         "ignored",
		},
	})
	details, ok := event.Details.(DegradedEventDetails)
	if !ok {
		t.Fatalf("expected typed degraded details, got %#v", event.Details)
	}
	if details.RecoveryStep != "binding-repair-then-portal-login" || details.Error != "still offline" {
		t.Fatalf("unexpected degraded details: %#v", details)
	}
}

func TestNormalizeEventKeepsTypedBindingRepairDetails(t *testing.T) {
	event := NormalizeEvent(Event{
		Kind: EventBindingRepair,
		Details: BindingRepairEventDetails{
			Action:        "moved",
			BindingOK:     true,
			HolderProfile: "W",
			RecoveryStep:  "binding-repair-then-portal-login",
			TargetProfile: "B",
		},
	})
	details, ok := event.Details.(BindingRepairEventDetails)
	if !ok {
		t.Fatalf("expected typed binding-repair details, got %#v", event.Details)
	}
	if !details.BindingOK || details.Action != "moved" || details.TargetProfile != "B" {
		t.Fatalf("unexpected binding-repair details: %#v", details)
	}
}
