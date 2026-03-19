package kernel

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestOpErrorUnwrap(t *testing.T) {
	err := &OpError{
		Op:      "self.login",
		Message: "login failed",
		Err:     ErrAuth,
	}
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("expected unwrap to expose ErrAuth, got %v", err)
	}
	if err.Error() == "" {
		t.Fatal("expected non-empty error string")
	}
}

func TestOpErrorErrorBranches(t *testing.T) {
	cases := []*OpError{
		{Op: "one", Message: "message only"},
		{Op: "two", Err: ErrPortal},
		{Op: "three"},
	}
	for _, tc := range cases {
		if tc.Error() == "" {
			t.Fatalf("expected non-empty error string for %#v", tc)
		}
	}
}

func TestCloneStateMap(t *testing.T) {
	original := map[string]string{"a": "1"}
	cloned := CloneStateMap(original)
	if cloned["a"] != "1" {
		t.Fatalf("unexpected clone: %#v", cloned)
	}
	cloned["a"] = "2"
	if original["a"] != "1" {
		t.Fatalf("expected original to remain unchanged: %#v", original)
	}
}

func TestCloneStateMapNil(t *testing.T) {
	if got := CloneStateMap(nil); got != nil {
		t.Fatalf("expected nil clone, got %#v", got)
	}
}

func TestProblemsFromError(t *testing.T) {
	err := &OpError{
		Op:      "portal.login802",
		Message: "ret_code=3 blocked",
		Err:     ErrPortalRetCode3,
		ProblemDetails: PortalProblemDetails{
			RetCode: "3",
		},
	}

	problems := ProblemsFromError(err)
	if len(problems) != 1 {
		t.Fatalf("expected one problem, got %#v", problems)
	}
	if problems[0].Code != ProblemPortalRetCode3 {
		t.Fatalf("unexpected problem code: %#v", problems[0])
	}
	details, ok := problems[0].Details.(PortalProblemDetails)
	if !ok {
		t.Fatalf("expected typed portal problem details, got %#v", problems[0].Details)
	}
	if details.RetCode != "3" {
		t.Fatalf("unexpected problem details: %#v", details)
	}
}

func TestMergeProblemsPreservesExisting(t *testing.T) {
	existing := []Problem{{Code: ProblemInvalidConfig, Message: "bad config"}}
	merged := MergeProblems(existing, ErrAuth)
	if len(merged) != 1 || merged[0].Code != ProblemInvalidConfig {
		t.Fatalf("unexpected merged problems: %#v", merged)
	}
}

func TestNormalizeProblemUpgradesLegacyPortalDetails(t *testing.T) {
	problem := NormalizeProblem(Problem{
		Code:    ProblemPortalRetCode3,
		Message: "blocked",
		Details: map[string]string{
			"retCode": "3",
			"noise":   "ignored",
		},
	})
	details, ok := problem.Details.(PortalProblemDetails)
	if !ok {
		t.Fatalf("expected typed portal problem details, got %#v", problem.Details)
	}
	if details.RetCode != "3" {
		t.Fatalf("expected retained retCode field, got %#v", details)
	}
}

func TestNormalizeProblemKeepsPortalAttemptDetails(t *testing.T) {
	problem := NormalizeProblem(Problem{
		Code:    ProblemPortalRequestFailed,
		Message: "transport failed",
		Details: map[string]any{
			"endpoint": "https://10.10.244.11:802/eportal/portal/login",
			"attempts": []any{
				map[string]any{
					"endpoint": "https://10.10.244.11:802/eportal/portal/login",
					"error":    "dial tcp timeout",
				},
				map[string]any{
					"endpoint": "https://p.njupt.edu.cn:802/eportal/portal/login",
					"error":    "lookup failed",
				},
			},
		},
	})

	details, ok := problem.Details.(PortalProblemDetails)
	if !ok {
		t.Fatalf("expected typed portal details, got %#v", problem.Details)
	}
	if len(details.Attempts) != 2 {
		t.Fatalf("expected portal attempts to survive normalization, got %#v", details)
	}
	if details.Attempts[1].Endpoint == "" || details.Attempts[1].Error == "" {
		t.Fatalf("unexpected portal attempt details: %#v", details)
	}
}

func TestNormalizeProblemUpgradesLegacyStateComparisonDetails(t *testing.T) {
	problem := NormalizeProblem(Problem{
		Code: ProblemReadbackMismatch,
		Details: map[string]any{
			"field":    "mobileAccount",
			"expected": "cmcc-user",
			"actual":   "",
		},
	})
	details, ok := problem.Details.(StateComparisonProblemDetails)
	if !ok {
		t.Fatalf("expected typed state-comparison details, got %#v", problem.Details)
	}
	if details.Field != "mobileAccount" || details.Expected != "cmcc-user" || details.Actual != "" {
		t.Fatalf("unexpected details: %#v", details)
	}
}

func TestNormalizeProblemUpgradesConfigDetails(t *testing.T) {
	problem := NormalizeProblem(Problem{
		Code: ProblemInvalidConfig,
		Details: map[string]any{
			"field": "guard.timezone",
			"value": "Mars/Base",
			"hint":  "use an IANA timezone",
		},
	})
	details, ok := problem.Details.(ConfigProblemDetails)
	if !ok {
		t.Fatalf("expected typed config details, got %#v", problem.Details)
	}
	if details.Field != "guard.timezone" || details.Value != "Mars/Base" || details.Hint != "use an IANA timezone" {
		t.Fatalf("unexpected config details: %#v", details)
	}
}

func TestNormalizeProblemUpgradesCapabilityDetails(t *testing.T) {
	problem := NormalizeProblem(Problem{
		Code: ProblemGuardedCapability,
		Details: map[string]string{
			"capability": "dashboard.offline",
			"reason":     "target session not present",
		},
	})
	details, ok := problem.Details.(CapabilityProblemDetails)
	if !ok {
		t.Fatalf("expected typed capability details, got %#v", problem.Details)
	}
	if details.Capability != "dashboard.offline" || details.Reason != "target session not present" {
		t.Fatalf("unexpected capability details: %#v", details)
	}
}

func TestOperatorBindingJSONKeepsEmptyFields(t *testing.T) {
	payload, err := json.Marshal(OperatorBinding{})
	if err != nil {
		t.Fatalf("marshal operator binding: %v", err)
	}
	want := `{"telecomAccount":"","telecomPassword":"","mobileAccount":"","mobilePassword":""}`
	if string(payload) != want {
		t.Fatalf("unexpected operator binding json:\n got %s\nwant %s", payload, want)
	}
}

func TestBillListResultJSONKeepsEmptyContainers(t *testing.T) {
	payload, err := json.Marshal(BillListResult{
		Summary: map[string]interface{}{},
		Total:   0,
		Rows:    []map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("marshal bill list: %v", err)
	}
	want := `{"summary":{},"total":0,"rows":[]}`
	if string(payload) != want {
		t.Fatalf("unexpected bill list json:\n got %s\nwant %s", payload, want)
	}
}
