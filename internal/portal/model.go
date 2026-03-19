package portal

import (
	"strings"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

func mapPortal802Response(payload map[string]any, endpoint, raw string) *kernel.Portal802Response {
	return &kernel.Portal802Response{
		Result:     toString(payload["result"]),
		RetCode:    toString(payload["ret_code"]),
		Msg:        toString(payload["msg"]),
		Endpoint:   endpoint,
		RawPayload: raw,
	}
}

func classifyRetCode(retCode string) (kernel.EvidenceLevel, error) {
	switch strings.TrimSpace(retCode) {
	case "1":
		return kernel.EvidenceGuarded, kernel.ErrPortalRetCode1
	case "3":
		return kernel.EvidenceBlocked, kernel.ErrPortalRetCode3
	case "8":
		return kernel.EvidenceBlocked, kernel.ErrPortalRetCode8
	case "":
		return kernel.EvidenceGuarded, kernel.ErrPortal
	default:
		return kernel.EvidenceGuarded, kernel.ErrPortalUnknownCode
	}
}

func rawCapture(resp *kernel.SessionResponse) *kernel.RawCapture {
	if resp == nil {
		return nil
	}
	return &kernel.RawCapture{
		Status:   resp.StatusCode,
		Headers:  resp.Headers,
		Body:     string(resp.Body),
		FinalURL: resp.FinalURL,
	}
}
