package selfservice

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

var mauthTogglePause = time.Second

// GetOnlineList returns the current online session list.
func (c *Client) GetOnlineList(ctx context.Context) (*kernel.OperationResult[[]kernel.OnlineSession], error) {
	if err := c.ensureSession("dashboard.onlineList"); err != nil {
		return nil, err
	}
	query := timestampQuery()
	query["order"] = "desc"
	resp, err := c.session.Get(ctx, "/Self/dashboard/getOnlineList", kernel.RequestOptions{Query: query})
	if err != nil {
		return nil, &kernel.OpError{Op: "dashboard.onlineList", Message: "request failed", Err: err}
	}

	var rawRows []map[string]interface{}
	if err := parseJSON(resp.Body, &rawRows); err != nil {
		return nil, &kernel.OpError{Op: "dashboard.onlineList", Message: "parse json array failed", Err: err}
	}

	sessions := parseOnlineSessions(rawRows)

	return &kernel.OperationResult[[]kernel.OnlineSession]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: fmt.Sprintf("loaded %d online sessions", len(sessions)),
		Data:    &sessions,
		Raw:     rawCapture(resp),
	}, nil
}

// GetLoginHistory returns the 2D history rows with stable columns named.
func (c *Client) GetLoginHistory(ctx context.Context) (*kernel.OperationResult[[]kernel.LoginHistoryEntry], error) {
	if err := c.ensureSession("dashboard.loginHistory"); err != nil {
		return nil, err
	}
	query := timestampQuery()
	query["order"] = "desc"
	resp, err := c.session.Get(ctx, "/Self/dashboard/getLoginHistory", kernel.RequestOptions{Query: query})
	if err != nil {
		return nil, &kernel.OpError{Op: "dashboard.loginHistory", Message: "request failed", Err: err}
	}

	var rows [][]interface{}
	if err := parseJSON(resp.Body, &rows); err != nil {
		return nil, &kernel.OpError{Op: "dashboard.loginHistory", Message: "parse 2D json failed", Err: err}
	}

	entries := parseLoginHistoryEntries(rows)

	return &kernel.OperationResult[[]kernel.LoginHistoryEntry]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: fmt.Sprintf("loaded %d login history rows", len(entries)),
		Data:    &entries,
		Raw:     rawCapture(resp),
	}, nil
}

// RefreshAccountRaw keeps the endpoint as a raw probe by design.
func (c *Client) RefreshAccountRaw(ctx context.Context) (*kernel.OperationResult[map[string]any], error) {
	if err := c.ensureSession("dashboard.refreshaccount"); err != nil {
		return nil, err
	}
	resp, err := c.session.Get(ctx, "/Self/dashboard/refreshaccount", kernel.RequestOptions{
		Query: map[string]string{"t": strconv.FormatInt(time.Now().UnixMilli(), 10)},
	})
	if err != nil {
		return nil, &kernel.OpError{Op: "dashboard.refreshaccount", Message: "request failed", Err: err}
	}
	data := map[string]any{"contentLength": len(resp.Body)}
	return &kernel.OperationResult[map[string]any]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: "refresh-account raw probe completed",
		Data:    &data,
		Raw:     rawCapture(resp),
	}, nil
}

// GetMauthState reads the current mauth fragment and maps it to the normalized state enum.
func (c *Client) GetMauthState(ctx context.Context) (*kernel.OperationResult[kernel.MauthState], error) {
	if err := c.ensureSession("dashboard.mauth.get"); err != nil {
		return nil, err
	}
	resp, err := c.session.Get(ctx, "/Self/dashboard/refreshMauthType", kernel.RequestOptions{
		Query: map[string]string{"t": strconv.FormatInt(time.Now().UnixMilli(), 10)},
	})
	if err != nil {
		return nil, &kernel.OpError{Op: "dashboard.mauth.get", Message: "request failed", Err: err}
	}
	if looksLikeLoginPage(resp.Body) {
		state := kernel.MauthUnknown
		return &kernel.OperationResult[kernel.MauthState]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: "session not authenticated",
			Data:    &state,
			Raw:     rawCapture(resp),
		}, &kernel.OpError{Op: "dashboard.mauth.get", Message: "refreshMauthType returned login page", Err: kernel.ErrAuth}
	}

	body := strings.TrimSpace(string(resp.Body))
	state := parseMauthState(body)

	return &kernel.OperationResult[kernel.MauthState]{
		Level:   kernel.EvidenceConfirmed,
		Success: state != kernel.MauthUnknown,
		Message: fmt.Sprintf("mauth state is %s", state),
		Data:    &state,
		Raw:     rawCapture(resp),
	}, nil
}

// ToggleMauth performs a verified state flip against the mauth toggle endpoint.
func (c *Client) ToggleMauth(ctx context.Context) (*kernel.OperationResult[kernel.MauthState], error) {
	before, err := c.GetMauthState(ctx)
	if err != nil {
		return nil, err
	}
	if before.Data == nil || *before.Data == kernel.MauthUnknown {
		return nil, &kernel.OpError{Op: "dashboard.mauth.toggle", Message: "current mauth state unknown", Err: kernel.ErrGuardedCapability, ProblemDetails: kernel.CapabilityProblemDetails{Capability: "dashboard.mauth.toggle", Reason: "current mauth state unknown"}}
	}

	resp, err := c.session.Get(ctx, "/Self/dashboard/oprateMauthAction", kernel.RequestOptions{})
	if err != nil {
		return nil, &kernel.OpError{Op: "dashboard.mauth.toggle", Message: "toggle request failed", Err: err}
	}
	time.Sleep(mauthTogglePause)

	after, err := c.GetMauthState(ctx)
	if err != nil {
		return nil, err
	}
	if after.Data == nil || *after.Data == *before.Data {
		beforeState := fmt.Sprint(*before.Data)
		afterState := "<nil>"
		if after.Data != nil {
			afterState = fmt.Sprint(*after.Data)
		}
		return &kernel.OperationResult[kernel.MauthState]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: "mauth state did not flip after toggle",
			Data:    after.Data,
			Raw:     rawCapture(resp),
			Problems: []kernel.Problem{kernel.NormalizeProblem(kernel.Problem{
				Code:    kernel.ProblemReadbackMismatch,
				Message: "mauth state did not flip after toggle",
				Details: kernel.StateComparisonProblemDetails{
					Before: beforeState,
					After:  afterState,
				},
			})},
		}, &kernel.OpError{Op: "dashboard.mauth.toggle", Message: "state not flipped", Err: kernel.ErrReadBackMismatch}
	}

	return &kernel.OperationResult[kernel.MauthState]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: fmt.Sprintf("mauth toggled to %s", *after.Data),
		Data:    after.Data,
		Raw:     rawCapture(resp),
	}, nil
}

// ForceOffline is guarded: it requires the target session to exist before firing.
func (c *Client) ForceOffline(ctx context.Context, sessionID string) (*kernel.OperationResult[map[string]any], error) {
	if err := c.ensureSession("dashboard.offline"); err != nil {
		return nil, err
	}
	pre, err := c.GetOnlineList(ctx)
	if err != nil {
		return nil, err
	}
	exists := false
	if pre.Data != nil {
		for _, row := range *pre.Data {
			if row.SessionID == sessionID {
				exists = true
				break
			}
		}
	}
	if !exists {
		payload := map[string]any{"sessionId": sessionID}
		return &kernel.OperationResult[map[string]any]{
			Level:   kernel.EvidenceGuarded,
			Success: false,
			Message: "target session not present in current online list",
			Data:    &payload,
		}, &kernel.OpError{Op: "dashboard.offline", Message: "session not found for guarded offline", Err: kernel.ErrGuardedCapability, ProblemDetails: kernel.CapabilityProblemDetails{Capability: "dashboard.offline", Reason: "target session not present"}}
	}

	resp, err := c.session.Get(ctx, "/Self/dashboard/tooffline", kernel.RequestOptions{Query: map[string]string{"sessionid": sessionID}})
	if err != nil {
		return nil, &kernel.OpError{Op: "dashboard.offline", Message: "tooffline request failed", Err: err}
	}

	var body map[string]interface{}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return nil, &kernel.OpError{Op: "dashboard.offline", Message: "parse offline response failed", Err: err}
	}

	post, err := c.GetOnlineList(ctx)
	if err != nil {
		return nil, err
	}
	stillExists := false
	if post.Data != nil {
		for _, row := range *post.Data {
			if row.SessionID == sessionID {
				stillExists = true
				break
			}
		}
	}

	data := map[string]any{
		"responseSuccess":     boolFromJSON(body["success"]),
		"sessionStillPresent": stillExists,
	}

	success := !stillExists
	message := "guarded offline request completed"
	if success {
		message = "session removed from online list after guarded offline request"
	}
	return &kernel.OperationResult[map[string]any]{
		Level:   kernel.EvidenceGuarded,
		Success: success,
		Message: message,
		Data:    &data,
		Raw:     rawCapture(resp),
	}, nil
}
