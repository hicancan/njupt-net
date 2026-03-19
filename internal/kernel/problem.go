package kernel

import "errors"

// ProblemCode is the stable machine-readable problem identifier for JSON output.
type ProblemCode string

const (
	ProblemAuthFailed              ProblemCode = "auth_failed"
	ProblemFreshLoginPageRequired  ProblemCode = "fresh_login_page_required"
	ProblemRandomCodeRequired      ProblemCode = "random_code_required"
	ProblemTokenExpired            ProblemCode = "token_expired"
	ProblemGuardedCapability       ProblemCode = "guarded_capability"
	ProblemBlockedCapability       ProblemCode = "blocked_capability"
	ProblemUnexpectedLoginRedirect ProblemCode = "unexpected_login_redirect"
	ProblemBusinessFailed          ProblemCode = "business_failed"
	ProblemPortalRequestFailed     ProblemCode = "portal_request_failed"
	ProblemPortalUnknownCode       ProblemCode = "portal_unknown_code"
	ProblemPortalRetCode1          ProblemCode = "portal_ret_code_1"
	ProblemPortalRetCode3          ProblemCode = "portal_ret_code_3"
	ProblemPortalRetCode8          ProblemCode = "portal_ret_code_8"
	ProblemPortalTLSFailure        ProblemCode = "portal_tls_failure"
	ProblemPortalFallbackRequired  ProblemCode = "portal_fallback_required"
	ProblemWriteNotObserved        ProblemCode = "write_not_observed"
	ProblemReadbackMismatch        ProblemCode = "readback_mismatch"
	ProblemRestoreFailed           ProblemCode = "restore_failed"
	ProblemInvalidConfig           ProblemCode = "invalid_config"
	ProblemUnknown                 ProblemCode = "unknown_error"
)

// Problem is the stable machine-readable problem envelope attached to results.
type Problem struct {
	Code    ProblemCode `json:"code"`
	Message string      `json:"message,omitempty"`
	Details any         `json:"details,omitempty"`
}

// ProblemCodeForError maps sentinel/runtime errors to stable problem codes.
func ProblemCodeForError(err error) ProblemCode {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrAuth):
		return ProblemAuthFailed
	case errors.Is(err, ErrNeedFreshLoginPage):
		return ProblemFreshLoginPageRequired
	case errors.Is(err, ErrNeedRandomCode):
		return ProblemRandomCodeRequired
	case errors.Is(err, ErrTokenExpired):
		return ProblemTokenExpired
	case errors.Is(err, ErrGuardedCapability):
		return ProblemGuardedCapability
	case errors.Is(err, ErrBlockedCapability):
		return ProblemBlockedCapability
	case errors.Is(err, ErrUnexpectedLoginRedirect):
		return ProblemUnexpectedLoginRedirect
	case errors.Is(err, ErrBusinessFailed):
		return ProblemBusinessFailed
	case errors.Is(err, ErrPortalUnknownCode):
		return ProblemPortalUnknownCode
	case errors.Is(err, ErrPortalRetCode1):
		return ProblemPortalRetCode1
	case errors.Is(err, ErrPortalRetCode3):
		return ProblemPortalRetCode3
	case errors.Is(err, ErrPortalRetCode8):
		return ProblemPortalRetCode8
	case errors.Is(err, ErrPortalTLS):
		return ProblemPortalTLSFailure
	case errors.Is(err, ErrPortalFallbackRequired):
		return ProblemPortalFallbackRequired
	case errors.Is(err, ErrPortal):
		return ProblemPortalRequestFailed
	case errors.Is(err, ErrWriteNotObserved):
		return ProblemWriteNotObserved
	case errors.Is(err, ErrReadBackMismatch):
		return ProblemReadbackMismatch
	case errors.Is(err, ErrRestoreFailed):
		return ProblemRestoreFailed
	case errors.Is(err, ErrInvalidConfig):
		return ProblemInvalidConfig
	default:
		return ProblemUnknown
	}
}

// ProblemsFromError converts an error into stable machine-readable problems.
func ProblemsFromError(err error) []Problem {
	if err == nil {
		return nil
	}
	var opErr *OpError
	if errors.As(err, &opErr) {
		if len(opErr.Problems) > 0 {
			return NormalizeProblems(opErr.Problems)
		}
		message := opErr.Message
		if message == "" {
			message = err.Error()
		}
		code := ProblemCodeForError(opErr.Err)
		if code == "" {
			code = ProblemCodeForError(err)
		}
		return NormalizeProblems([]Problem{{
			Code:    code,
			Message: message,
			Details: opErr.ProblemDetails,
		}})
	}
	return NormalizeProblems([]Problem{{
		Code:    ProblemCodeForError(err),
		Message: err.Error(),
	}})
}

// MergeProblems preserves existing result problems and falls back to the error mapping when empty.
func MergeProblems(existing []Problem, err error) []Problem {
	if len(existing) > 0 {
		return NormalizeProblems(existing)
	}
	return ProblemsFromError(err)
}
