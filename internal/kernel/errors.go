package kernel

import "errors"

var (
	ErrAuth                    = errors.New("authentication failed")
	ErrNeedFreshLoginPage      = errors.New("fresh login page required")
	ErrNeedRandomCode          = errors.New("randomCode prerequisite required")
	ErrTokenExpired            = errors.New("token missing or expired")
	ErrGuardedCapability       = errors.New("guarded capability")
	ErrBlockedCapability       = errors.New("blocked capability")
	ErrUnexpectedLoginRedirect = errors.New("unexpected login redirect")
	ErrBusinessFailed          = errors.New("business verification failed")
	ErrPortal                  = errors.New("portal request failed")
	ErrPortalUnknownCode       = errors.New("unknown portal ret_code")
	ErrPortalRetCode1          = errors.New("portal ret_code=1")
	ErrPortalRetCode3          = errors.New("portal ret_code=3")
	ErrPortalRetCode8          = errors.New("portal ret_code=8")
	ErrPortalTLS               = errors.New("portal tls failure")
	ErrPortalFallbackRequired  = errors.New("portal fallback required")
	ErrWriteNotObserved        = errors.New("write not observed")
	ErrReadBackMismatch        = errors.New("readback mismatch")
	ErrRestoreFailed           = errors.New("restore failed")
	ErrInvalidConfig           = errors.New("invalid config")
)

// OpError decorates a sentinel error with operation-scoped context.
type OpError struct {
	Op             string
	Message        string
	Err            error
	ProblemDetails any
	Problems       []Problem
}

func (e *OpError) Error() string {
	if e == nil {
		return "operation error"
	}
	switch {
	case e.Message != "" && e.Err != nil:
		return e.Op + ": " + e.Message + ": " + e.Err.Error()
	case e.Message != "":
		return e.Op + ": " + e.Message
	case e.Err != nil:
		return e.Op + ": " + e.Err.Error()
	default:
		return e.Op + ": operation error"
	}
}

func (e *OpError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
