package kernel

import (
	"fmt"
	"strings"
)

// PortalProblemDetails is the typed machine-readable payload for Portal failures.
type PortalProblemDetails struct {
	Endpoint string                `json:"endpoint,omitempty"`
	Msg      string                `json:"msg,omitempty"`
	Result   string                `json:"result,omitempty"`
	RetCode  string                `json:"retCode,omitempty"`
	Attempts []PortalAttemptDetail `json:"attempts,omitempty"`
}

// PortalAttemptDetail captures one concrete portal endpoint attempt.
type PortalAttemptDetail struct {
	Endpoint string `json:"endpoint,omitempty"`
	Error    string `json:"error,omitempty"`
}

// StateComparisonProblemDetails captures readback/restore mismatch evidence.
type StateComparisonProblemDetails struct {
	Field    string `json:"field,omitempty"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Before   string `json:"before,omitempty"`
	After    string `json:"after,omitempty"`
}

// StringProblemDetails is a simple typed single-field problem payload.
type StringProblemDetails struct {
	Value string `json:"value,omitempty"`
}

// ConfigProblemDetails is the typed machine-readable payload for configuration errors.
type ConfigProblemDetails struct {
	Field string `json:"field,omitempty"`
	Hint  string `json:"hint,omitempty"`
	Value string `json:"value,omitempty"`
}

// CapabilityProblemDetails is the typed payload for guarded or blocked capabilities.
type CapabilityProblemDetails struct {
	Capability string `json:"capability,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// NormalizeProblem upgrades problem details to the supported typed payloads.
func NormalizeProblem(problem Problem) Problem {
	if problem.Code == "" {
		problem.Code = ProblemUnknown
	}
	problem.Details = normalizeProblemDetails(problem.Code, problem.Details)
	return problem
}

// NormalizeProblems normalizes and clones problem slices for stable machine output.
func NormalizeProblems(problems []Problem) []Problem {
	if len(problems) == 0 {
		return nil
	}
	out := make([]Problem, 0, len(problems))
	for _, problem := range problems {
		out = append(out, NormalizeProblem(problem))
	}
	return out
}

func normalizeProblemDetails(code ProblemCode, details any) any {
	switch code {
	case ProblemPortalRequestFailed, ProblemPortalUnknownCode, ProblemPortalRetCode1, ProblemPortalRetCode3, ProblemPortalRetCode8, ProblemPortalTLSFailure, ProblemPortalFallbackRequired:
		return normalizePortalProblemDetails(details)
	case ProblemReadbackMismatch, ProblemRestoreFailed:
		return normalizeStateComparisonProblemDetails(details)
	case ProblemInvalidConfig:
		return normalizeConfigProblemDetails(details)
	case ProblemGuardedCapability, ProblemBlockedCapability:
		return normalizeCapabilityProblemDetails(details)
	default:
		return details
	}
}

func normalizePortalProblemDetails(details any) any {
	switch value := details.(type) {
	case nil:
		return nil
	case PortalProblemDetails:
		return value
	case *PortalProblemDetails:
		if value == nil {
			return nil
		}
		return *value
	case map[string]string:
		return PortalProblemDetails{
			Endpoint: value["endpoint"],
			Msg:      value["msg"],
			Result:   value["result"],
			RetCode:  value["retCode"],
		}
	case map[string]any:
		details := PortalProblemDetails{
			Endpoint: toString(value["endpoint"]),
			Msg:      toString(value["msg"]),
			Result:   toString(value["result"]),
			RetCode:  toString(value["retCode"]),
		}
		if attempts, ok := value["attempts"].([]any); ok {
			details.Attempts = make([]PortalAttemptDetail, 0, len(attempts))
			for _, attempt := range attempts {
				if mapped, ok := attempt.(map[string]any); ok {
					details.Attempts = append(details.Attempts, PortalAttemptDetail{
						Endpoint: toString(mapped["endpoint"]),
						Error:    toString(mapped["error"]),
					})
				}
			}
		}
		return details
	default:
		return nil
	}
}

func normalizeStateComparisonProblemDetails(details any) any {
	switch value := details.(type) {
	case nil:
		return nil
	case StateComparisonProblemDetails:
		return value
	case *StateComparisonProblemDetails:
		if value == nil {
			return nil
		}
		return *value
	case map[string]string:
		return StateComparisonProblemDetails{
			Field:    value["field"],
			Expected: value["expected"],
			Actual:   value["actual"],
			Before:   value["before"],
			After:    value["after"],
		}
	case map[string]any:
		return StateComparisonProblemDetails{
			Field:    toString(value["field"]),
			Expected: toString(value["expected"]),
			Actual:   toString(value["actual"]),
			Before:   toString(value["before"]),
			After:    toString(value["after"]),
		}
	default:
		return nil
	}
}

func normalizeConfigProblemDetails(details any) any {
	switch value := details.(type) {
	case nil:
		return nil
	case ConfigProblemDetails:
		return value
	case *ConfigProblemDetails:
		if value == nil {
			return nil
		}
		return *value
	case map[string]string:
		return ConfigProblemDetails{
			Field: value["field"],
			Hint:  value["hint"],
			Value: value["value"],
		}
	case map[string]any:
		return ConfigProblemDetails{
			Field: toString(value["field"]),
			Hint:  toString(value["hint"]),
			Value: toString(value["value"]),
		}
	default:
		return nil
	}
}

func normalizeCapabilityProblemDetails(details any) any {
	switch value := details.(type) {
	case nil:
		return nil
	case CapabilityProblemDetails:
		return value
	case *CapabilityProblemDetails:
		if value == nil {
			return nil
		}
		return *value
	case map[string]string:
		return CapabilityProblemDetails{
			Capability: value["capability"],
			Reason:     value["reason"],
		}
	case map[string]any:
		return CapabilityProblemDetails{
			Capability: toString(value["capability"]),
			Reason:     toString(value["reason"]),
		}
	default:
		return nil
	}
}

func toString(value any) string {
	return strings.TrimSpace(fmt.Sprint(value))
}
