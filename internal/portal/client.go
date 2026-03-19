package portal

import (
	"context"
	"fmt"
	"strings"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

const (
	default802BaseURL = "https://10.10.244.11:802/eportal/portal"
	default801BaseURL = "http://p.njupt.edu.cn:801/eportal/"
	jsonpCallback     = "dr1003"
)

// Client implements Portal 802 as primary and 801 as guarded fallback.
type Client struct {
	session            kernel.SessionClient
	baseURL802         string
	fallbackBaseURL802 string
	baseURL801         string
}

func NewClient(session kernel.SessionClient, baseURL802, fallbackBaseURL802 string) *Client {
	if strings.TrimSpace(baseURL802) == "" {
		baseURL802 = default802BaseURL
	}
	if strings.TrimSpace(fallbackBaseURL802) == "" {
		fallbackBaseURL802 = "https://p.njupt.edu.cn:802/eportal/portal"
	}
	return &Client{
		session:            session,
		baseURL802:         strings.TrimRight(baseURL802, "/"),
		fallbackBaseURL802: strings.TrimRight(fallbackBaseURL802, "/"),
		baseURL801:         strings.TrimRight(default801BaseURL, "/"),
	}
}

// Login802 performs one 802 login attempt, retrying on transport failure via the fallback host.
func (c *Client) Login802(ctx context.Context, account, password, ip, isp string) (*kernel.OperationResult[kernel.Portal802Response], error) {
	if c == nil || c.session == nil {
		return nil, &kernel.OpError{Op: "portal.login802", Message: "session client is nil", Err: kernel.ErrPortal}
	}
	endpoints := []string{c.baseURL802}
	if c.fallbackBaseURL802 != "" && c.fallbackBaseURL802 != c.baseURL802 {
		endpoints = append(endpoints, c.fallbackBaseURL802)
	}

	var lastErr error
	for _, endpoint := range endpoints {
		result, err := c.login802Once(ctx, endpoint, account, password, ip, isp)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if result != nil && result.Raw != nil {
			return result, err
		}
	}
	return nil, lastErr
}

// Logout802 sends the 802 logout call and treats transport success as success.
func (c *Client) Logout802(ctx context.Context, ip string) (*kernel.OperationResult[kernel.Portal802Response], error) {
	if c == nil || c.session == nil {
		return nil, &kernel.OpError{Op: "portal.logout802", Message: "session client is nil", Err: kernel.ErrPortal}
	}
	resp, err := c.session.Get(ctx, c.baseURL802+"/logout", kernel.RequestOptions{
		Query: buildLogout802Query(ip),
	})
	if err != nil {
		return nil, &kernel.OpError{Op: "portal.logout802", Message: "request failed", Err: err}
	}
	data := &kernel.Portal802Response{
		Endpoint:   c.baseURL802 + "/logout",
		RawPayload: string(resp.Body),
	}
	return &kernel.OperationResult[kernel.Portal802Response]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: "portal 802 logout request completed",
		Data:    data,
		Raw:     rawCapture(resp),
	}, nil
}

// Login801 performs a guarded raw fallback attempt.
func (c *Client) Login801(ctx context.Context, account, password, ip, ipv6 string) (*kernel.OperationResult[map[string]any], error) {
	if c == nil || c.session == nil {
		return nil, &kernel.OpError{Op: "portal.login801", Message: "session client is nil", Err: kernel.ErrPortal}
	}
	resp, err := c.session.Get(ctx, c.baseURL801, kernel.RequestOptions{Query: buildLogin801Query(account, password, ip, ipv6)})
	if err != nil {
		return nil, &kernel.OpError{Op: "portal.login801", Message: "request failed", Err: err}
	}
	data := map[string]any{"endpoint": c.baseURL801, "bodyLength": len(resp.Body)}
	return &kernel.OperationResult[map[string]any]{
		Level:   kernel.EvidenceGuarded,
		Success: false,
		Message: "portal 801 fallback completed as raw guarded probe",
		Data:    &data,
		Raw:     rawCapture(resp),
	}, &kernel.OpError{Op: "portal.login801", Message: "801 cannot determine success semantics from body", Err: kernel.ErrPortalFallbackRequired}
}

// Logout801 performs a guarded raw fallback logout.
func (c *Client) Logout801(ctx context.Context, ip string) (*kernel.OperationResult[map[string]any], error) {
	if c == nil || c.session == nil {
		return nil, &kernel.OpError{Op: "portal.logout801", Message: "session client is nil", Err: kernel.ErrPortal}
	}
	resp, err := c.session.Get(ctx, c.baseURL801, kernel.RequestOptions{
		Query: buildLogout801Query(ip),
	})
	if err != nil {
		return nil, &kernel.OpError{Op: "portal.logout801", Message: "request failed", Err: err}
	}
	data := map[string]any{"endpoint": c.baseURL801, "bodyLength": len(resp.Body)}
	return &kernel.OperationResult[map[string]any]{
		Level:   kernel.EvidenceGuarded,
		Success: false,
		Message: "portal 801 logout completed as raw guarded probe",
		Data:    &data,
		Raw:     rawCapture(resp),
	}, &kernel.OpError{Op: "portal.logout801", Message: "801 cannot determine success semantics from body", Err: kernel.ErrPortalFallbackRequired}
}

func (c *Client) login802Once(ctx context.Context, endpoint, account, password, ip, isp string) (*kernel.OperationResult[kernel.Portal802Response], error) {
	resp, err := c.session.Get(ctx, endpoint+"/login", kernel.RequestOptions{
		Query: buildLogin802Query(account, password, ip, isp),
	})
	if err != nil {
		return nil, &kernel.OpError{Op: "portal.login802", Message: fmt.Sprintf("transport failed for %s", endpoint), Err: err}
	}

	payload, parseErr := parseJSONPPayload(string(resp.Body))
	if parseErr != nil {
		return &kernel.OperationResult[kernel.Portal802Response]{
			Level:   kernel.EvidenceGuarded,
			Success: false,
			Message: "invalid portal 802 JSONP payload",
			Raw:     rawCapture(resp),
		}, &kernel.OpError{Op: "portal.login802", Message: "invalid jsonp payload", Err: parseErr}
	}

	result := mapPortal802Response(payload, endpoint+"/login", string(resp.Body))
	opResult := &kernel.OperationResult[kernel.Portal802Response]{
		Data: result,
		Raw:  rawCapture(resp),
	}

	if result.Result == "1" {
		opResult.Level = kernel.EvidenceConfirmed
		opResult.Success = true
		opResult.Message = "portal 802 login succeeded"
		return opResult, nil
	}

	level, sentinel := classifyRetCode(result.RetCode)
	opResult.Level = level
	opResult.Success = false
	opResult.Message = fmt.Sprintf("portal 802 login failed ret_code=%s msg=%s", result.RetCode, result.Msg)
	return opResult, &kernel.OpError{
		Op:      "portal.login802",
		Message: opResult.Message,
		Err:     sentinel,
		ProblemDetails: kernel.PortalProblemDetails{
			RetCode:  result.RetCode,
			Msg:      result.Msg,
			Result:   result.Result,
			Endpoint: result.Endpoint,
		},
	}
}
