package kernel

import (
	"encoding/json"
	"testing"
)

func TestProblemJSONContract(t *testing.T) {
	problem := NormalizeProblem(Problem{
		Code:    ProblemPortalRetCode3,
		Message: "portal blocked",
		Details: PortalProblemDetails{
			Endpoint: "https://10.10.244.11:802/eportal/portal",
			RetCode:  "3",
		},
	})
	payload, err := json.Marshal(problem)
	if err != nil {
		t.Fatalf("marshal problem: %v", err)
	}
	want := `{"code":"portal_ret_code_3","message":"portal blocked","details":{"endpoint":"https://10.10.244.11:802/eportal/portal","retCode":"3"}}`
	if string(payload) != want {
		t.Fatalf("unexpected problem json:\n got %s\nwant %s", payload, want)
	}
}

func TestConfigProblemJSONContract(t *testing.T) {
	problem := NormalizeProblem(Problem{
		Code:    ProblemInvalidConfig,
		Message: "invalid guard timezone",
		Details: ConfigProblemDetails{
			Field: "guard.timezone",
			Hint:  "use an IANA timezone",
			Value: "Mars/Base",
		},
	})
	payload, err := json.Marshal(problem)
	if err != nil {
		t.Fatalf("marshal config problem: %v", err)
	}
	want := `{"code":"invalid_config","message":"invalid guard timezone","details":{"field":"guard.timezone","hint":"use an IANA timezone","value":"Mars/Base"}}`
	if string(payload) != want {
		t.Fatalf("unexpected config problem json:\n got %s\nwant %s", payload, want)
	}
}

func TestCapabilityProblemJSONContract(t *testing.T) {
	problem := NormalizeProblem(Problem{
		Code:    ProblemGuardedCapability,
		Message: "dashboard offline is guarded",
		Details: CapabilityProblemDetails{
			Capability: "dashboard.offline",
			Reason:     "target session not present",
		},
	})
	payload, err := json.Marshal(problem)
	if err != nil {
		t.Fatalf("marshal capability problem: %v", err)
	}
	want := `{"code":"guarded_capability","message":"dashboard offline is guarded","details":{"capability":"dashboard.offline","reason":"target session not present"}}`
	if string(payload) != want {
		t.Fatalf("unexpected capability problem json:\n got %s\nwant %s", payload, want)
	}
}
