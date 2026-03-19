package kernel

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestProblemCodeForErrorCoversKnownSentinels(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want ProblemCode
	}{
		{name: "nil", err: nil, want: ""},
		{name: "auth", err: ErrAuth, want: ProblemAuthFailed},
		{name: "fresh-login", err: ErrNeedFreshLoginPage, want: ProblemFreshLoginPageRequired},
		{name: "random-code", err: ErrNeedRandomCode, want: ProblemRandomCodeRequired},
		{name: "token-expired", err: ErrTokenExpired, want: ProblemTokenExpired},
		{name: "guarded", err: ErrGuardedCapability, want: ProblemGuardedCapability},
		{name: "blocked", err: ErrBlockedCapability, want: ProblemBlockedCapability},
		{name: "unexpected-redirect", err: ErrUnexpectedLoginRedirect, want: ProblemUnexpectedLoginRedirect},
		{name: "business", err: ErrBusinessFailed, want: ProblemBusinessFailed},
		{name: "portal", err: ErrPortal, want: ProblemPortalRequestFailed},
		{name: "portal-unknown", err: ErrPortalUnknownCode, want: ProblemPortalUnknownCode},
		{name: "portal-1", err: ErrPortalRetCode1, want: ProblemPortalRetCode1},
		{name: "portal-3", err: ErrPortalRetCode3, want: ProblemPortalRetCode3},
		{name: "portal-8", err: ErrPortalRetCode8, want: ProblemPortalRetCode8},
		{name: "portal-tls", err: ErrPortalTLS, want: ProblemPortalTLSFailure},
		{name: "portal-fallback", err: ErrPortalFallbackRequired, want: ProblemPortalFallbackRequired},
		{name: "write-not-observed", err: ErrWriteNotObserved, want: ProblemWriteNotObserved},
		{name: "readback-mismatch", err: ErrReadBackMismatch, want: ProblemReadbackMismatch},
		{name: "restore-failed", err: ErrRestoreFailed, want: ProblemRestoreFailed},
		{name: "invalid-config", err: ErrInvalidConfig, want: ProblemInvalidConfig},
		{name: "unknown", err: errors.New("boom"), want: ProblemUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ProblemCodeForError(tc.err); got != tc.want {
				t.Fatalf("unexpected problem code: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestProblemsFromErrorUsesEmbeddedProblemsWhenPresent(t *testing.T) {
	err := &OpError{
		Op:      "service.binding.set",
		Message: "readback mismatch",
		Err:     ErrReadBackMismatch,
		Problems: []Problem{
			{
				Code:    ProblemReadbackMismatch,
				Message: "readback mismatch",
				Details: map[string]any{"field": "operatorId", "expected": "mobile", "actual": "telecom"},
			},
		},
	}

	problems := ProblemsFromError(err)
	if len(problems) != 1 {
		t.Fatalf("expected one problem, got %#v", problems)
	}
	details, ok := problems[0].Details.(StateComparisonProblemDetails)
	if !ok {
		t.Fatalf("expected typed state comparison details, got %#v", problems[0].Details)
	}
	if details.Field != "operatorId" || details.Expected != "mobile" || details.Actual != "telecom" {
		t.Fatalf("unexpected details: %#v", details)
	}
}

func TestNormalizeProblemHandlesPointersAndUnsupportedDetails(t *testing.T) {
	cases := []struct {
		name     string
		problem  Problem
		wantType string
	}{
		{
			name: "portal-pointer",
			problem: Problem{
				Code:    ProblemPortalRetCode1,
				Details: &PortalProblemDetails{Endpoint: "https://10.10.244.11:802/eportal/portal", RetCode: "1"},
			},
			wantType: "portal",
		},
		{
			name: "state-pointer",
			problem: Problem{
				Code:    ProblemRestoreFailed,
				Details: &StateComparisonProblemDetails{Field: "mobileAccount", Before: "a", After: "b"},
			},
			wantType: "state",
		},
		{
			name: "config-pointer",
			problem: Problem{
				Code:    ProblemInvalidConfig,
				Details: &ConfigProblemDetails{Field: "guard.timezone", Value: "Mars/Base"},
			},
			wantType: "config",
		},
		{
			name: "capability-pointer",
			problem: Problem{
				Code:    ProblemBlockedCapability,
				Details: &CapabilityProblemDetails{Capability: "setting.person.update", Reason: "environment-blocked"},
			},
			wantType: "capability",
		},
		{
			name: "unknown-code-defaults",
			problem: Problem{
				Message: "unknown",
				Details: map[string]string{"value": "ignored"},
			},
			wantType: "unknown",
		},
		{
			name: "unsupported-detail-type-becomes-nil",
			problem: Problem{
				Code:    ProblemPortalRequestFailed,
				Details: 17,
			},
			wantType: "nil",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			normalized := NormalizeProblem(tc.problem)
			switch tc.wantType {
			case "portal":
				if _, ok := normalized.Details.(PortalProblemDetails); !ok {
					t.Fatalf("expected typed portal details, got %#v", normalized.Details)
				}
			case "state":
				if _, ok := normalized.Details.(StateComparisonProblemDetails); !ok {
					t.Fatalf("expected typed state details, got %#v", normalized.Details)
				}
			case "config":
				if _, ok := normalized.Details.(ConfigProblemDetails); !ok {
					t.Fatalf("expected typed config details, got %#v", normalized.Details)
				}
			case "capability":
				if _, ok := normalized.Details.(CapabilityProblemDetails); !ok {
					t.Fatalf("expected typed capability details, got %#v", normalized.Details)
				}
			case "unknown":
				if normalized.Code != ProblemUnknown {
					t.Fatalf("expected unknown code, got %#v", normalized.Code)
				}
			case "nil":
				if normalized.Details != nil {
					t.Fatalf("expected nil details, got %#v", normalized.Details)
				}
			}
		})
	}
}

func TestProblemJSONContractsForStateComparisonAndBlockedFamilies(t *testing.T) {
	stateProblem := NormalizeProblem(Problem{
		Code:    ProblemReadbackMismatch,
		Message: "binding readback mismatch",
		Details: StateComparisonProblemDetails{
			Field:    "operatorId",
			Expected: "mobile",
			Actual:   "telecom",
		},
	})
	statePayload, err := json.Marshal(stateProblem)
	if err != nil {
		t.Fatalf("marshal state problem: %v", err)
	}
	stateWant := `{"code":"readback_mismatch","message":"binding readback mismatch","details":{"field":"operatorId","expected":"mobile","actual":"telecom"}}`
	if string(statePayload) != stateWant {
		t.Fatalf("unexpected state comparison problem json:\n got %s\nwant %s", statePayload, stateWant)
	}

	blockedProblem := NormalizeProblem(Problem{
		Code:    ProblemBlockedCapability,
		Message: "setting person update is blocked",
		Details: CapabilityProblemDetails{
			Capability: "setting.person.update",
			Reason:     "submit semantics intentionally blocked",
		},
	})
	blockedPayload, err := json.Marshal(blockedProblem)
	if err != nil {
		t.Fatalf("marshal blocked problem: %v", err)
	}
	blockedWant := `{"code":"blocked_capability","message":"setting person update is blocked","details":{"capability":"setting.person.update","reason":"submit semantics intentionally blocked"}}`
	if string(blockedPayload) != blockedWant {
		t.Fatalf("unexpected blocked problem json:\n got %s\nwant %s", blockedPayload, blockedWant)
	}
}
