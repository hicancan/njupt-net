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
var (
	offlineReadbackPause    = time.Second
	offlineReadbackAttempts = 3
)

func waitDashboardDelay(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

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
		Raw:     kernel.CaptureRaw(resp),
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
		Raw:     kernel.CaptureRaw(resp),
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
		Raw:     kernel.CaptureRaw(resp),
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
	if responseLooksLikeLogin(resp) {
		state := kernel.MauthUnknown
		return &kernel.OperationResult[kernel.MauthState]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: "session not authenticated",
			Data:    &state,
			Raw:     kernel.CaptureRaw(resp),
		}, &kernel.OpError{Op: "dashboard.mauth.get", Message: "refreshMauthType returned login page", Err: kernel.ErrAuth}
	}

	body := strings.TrimSpace(string(resp.Body))
	state := parseMauthState(body)

	return &kernel.OperationResult[kernel.MauthState]{
		Level:   kernel.EvidenceConfirmed,
		Success: state != kernel.MauthUnknown,
		Message: fmt.Sprintf("mauth state is %s", state),
		Data:    &state,
		Raw:     kernel.CaptureRaw(resp),
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
	if err := waitDashboardDelay(ctx, mauthTogglePause); err != nil {
		return nil, err
	}

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
			Raw:     kernel.CaptureRaw(resp),
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
		Raw:     kernel.CaptureRaw(resp),
	}, nil
}

// ForceOffline removes a specific online session and confirms success by bounded readback.
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

	stillExists := true
	replacementDetected := false
	attempts := offlineReadbackAttempts
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 && offlineReadbackPause > 0 {
			if err := waitDashboardDelay(ctx, offlineReadbackPause); err != nil {
				return nil, err
			}
		}
		post, err := c.GetOnlineList(ctx)
		if err != nil {
			return nil, err
		}
		stillExists, replacementDetected = offlineSessionState(post.Data, sessionID)
		if !stillExists {
			data := map[string]any{
				"responseSuccess":            boolFromJSON(body["success"]),
				"sessionStillPresent":        false,
				"replacementSessionDetected": replacementDetected,
				"verificationAttempts":       attempt,
			}
			message := "target session removed from online list after offline request"
			if replacementDetected {
				message = "target session removed from online list; follow-up session detected"
			}
			return &kernel.OperationResult[map[string]any]{
				Level:   kernel.EvidenceConfirmed,
				Success: true,
				Message: message,
				Data:    &data,
				Raw:     kernel.CaptureRaw(resp),
			}, nil
		}
	}

	data := map[string]any{
		"responseSuccess":            boolFromJSON(body["success"]),
		"sessionStillPresent":        true,
		"replacementSessionDetected": replacementDetected,
		"verificationAttempts":       attempts,
	}
	return &kernel.OperationResult[map[string]any]{
			Level:   kernel.EvidenceGuarded,
			Success: false,
			Message: "target session still present after offline request verification",
			Data:    &data,
			Raw:     kernel.CaptureRaw(resp),
			Problems: []kernel.Problem{kernel.NormalizeProblem(kernel.Problem{
				Code:    kernel.ProblemReadbackMismatch,
				Message: "target session still present after offline request verification",
				Details: kernel.StateComparisonProblemDetails{
					Field:    "sessionId",
					Expected: "<removed>",
					Actual:   sessionID,
				},
			})},
		}, &kernel.OpError{
			Op:      "dashboard.offline",
			Message: "target session still present after offline request verification",
			Err:     kernel.ErrReadBackMismatch,
			ProblemDetails: kernel.StateComparisonProblemDetails{
				Field:    "sessionId",
				Expected: "<removed>",
				Actual:   sessionID,
			},
		}
}

func offlineSessionState(data *[]kernel.OnlineSession, sessionID string) (stillExists bool, replacementDetected bool) {
	if data == nil {
		return false, false
	}
	for _, row := range *data {
		if row.SessionID == sessionID {
			stillExists = true
			continue
		}
		if strings.TrimSpace(row.SessionID) != "" {
			replacementDetected = true
		}
	}
	return stillExists, replacementDetected
}
