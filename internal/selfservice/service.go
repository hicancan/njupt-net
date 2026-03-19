package selfservice

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

var (
	bindFields             = []string{"FLDEXTRA1", "FLDEXTRA2", "FLDEXTRA3", "FLDEXTRA4"}
	installmentFlagPattern = regexp.MustCompile(`(?i)installmentFlag["']?\s*[:=]\s*["']?([^"',}<\s]+)`)
)

// GetOperatorBinding reads the current binding state page.
func (c *Client) GetOperatorBinding(ctx context.Context) (*kernel.OperationResult[kernel.OperatorBinding], error) {
	if err := c.ensureSession("service.binding.get"); err != nil {
		return nil, err
	}
	_, state, resp, err := c.getOperatorState(ctx)
	if err != nil {
		return nil, err
	}
	binding := operatorBindingFromState(state)
	return &kernel.OperationResult[kernel.OperatorBinding]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: "operator binding loaded",
		Data:    &binding,
		Raw:     rawCapture(resp),
	}, nil
}

// BindOperator applies target FLDEXTRA values and verifies readback.
func (c *Client) BindOperator(ctx context.Context, target map[string]string, readback, restore bool) (*kernel.OperationResult[kernel.WriteBackResult], error) {
	if err := c.ensureSession("service.binding.set"); err != nil {
		return nil, err
	}
	if len(target) == 0 {
		return nil, &kernel.OpError{Op: "service.binding.set", Message: "at least one FLDEXTRA field is required", Err: kernel.ErrBusinessFailed}
	}

	token, preState, _, err := c.getOperatorState(ctx)
	if err != nil {
		return nil, err
	}

	form := kernel.CloneStateMap(preState)
	form["csrftoken"] = token
	for field, value := range target {
		form[field] = value
	}
	resp, err := c.session.PostForm(ctx, bindOperatorPath, kernel.RequestOptions{Form: form})
	if err != nil {
		return nil, &kernel.OpError{Op: "service.binding.set", Message: "submit failed", Err: err}
	}

	writeResult := &kernel.WriteBackResult{
		PreState:    kernel.CloneStateMap(preState),
		TargetState: kernel.CloneStateMap(target),
	}

	if readback {
		_, postState, _, err := c.getOperatorState(ctx)
		if err != nil {
			return nil, err
		}
		writeResult.PostState = kernel.CloneStateMap(postState)
		for field, expected := range target {
			if postState[field] != expected {
				return &kernel.OperationResult[kernel.WriteBackResult]{
						Level:   kernel.EvidenceConfirmed,
						Success: false,
						Message: fmt.Sprintf("binding readback mismatch for %s", field),
						Data:    writeResult,
						Raw:     rawCapture(resp),
					}, &kernel.OpError{
						Op:      "service.binding.set",
						Message: fmt.Sprintf("%s expected=%q got=%q", field, expected, postState[field]),
						Err:     kernel.ErrReadBackMismatch,
						ProblemDetails: kernel.StateComparisonProblemDetails{
							Field:    field,
							Expected: expected,
							Actual:   postState[field],
						},
					}
			}
		}
	}

	if restore {
		restoreForm := kernel.CloneStateMap(preState)
		restoreForm["csrftoken"] = token
		if _, err := c.session.PostForm(ctx, bindOperatorPath, kernel.RequestOptions{Form: restoreForm}); err != nil {
			return nil, &kernel.OpError{Op: "service.binding.restore", Message: "restore submit failed", Err: err}
		}
		_, restored, _, err := c.getOperatorState(ctx)
		if err != nil {
			return nil, err
		}
		writeResult.RestoredState = kernel.CloneStateMap(restored)
		for field, expected := range preState {
			if restored[field] != expected {
				return &kernel.OperationResult[kernel.WriteBackResult]{
						Level:   kernel.EvidenceConfirmed,
						Success: false,
						Message: "binding restore failed",
						Data:    writeResult,
						Raw:     rawCapture(resp),
					}, &kernel.OpError{
						Op:      "service.binding.restore",
						Message: fmt.Sprintf("%s expected=%q got=%q", field, expected, restored[field]),
						Err:     kernel.ErrRestoreFailed,
						ProblemDetails: kernel.StateComparisonProblemDetails{
							Field:    field,
							Expected: expected,
							Actual:   restored[field],
						},
					}
			}
		}
	}

	return &kernel.OperationResult[kernel.WriteBackResult]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: "operator binding updated",
		Data:    writeResult,
		Raw:     rawCapture(resp),
	}, nil
}

// GetConsumeProtect parses the consumeProtect page into a typed state.
func (c *Client) GetConsumeProtect(ctx context.Context) (*kernel.OperationResult[kernel.ConsumeProtectState], error) {
	if err := c.ensureSession("service.consume.get"); err != nil {
		return nil, err
	}
	state, resp, err := c.readConsumeProtect(ctx)
	if err != nil {
		return nil, err
	}
	return &kernel.OperationResult[kernel.ConsumeProtectState]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: "consume protect state loaded",
		Data:    state,
		Raw:     rawCapture(resp),
	}, nil
}

// ChangeConsumeProtect writes consumeLimit and verifies installmentFlag readback.
func (c *Client) ChangeConsumeProtect(ctx context.Context, limit string, readback, restore bool) (*kernel.OperationResult[kernel.WriteBackResult], error) {
	if err := c.ensureSession("service.consume.set"); err != nil {
		return nil, err
	}
	state, _, err := c.readConsumeProtect(ctx)
	if err != nil {
		return nil, err
	}
	form := map[string]string{
		"csrftoken":    state.CSRFTOKEN,
		"consumeLimit": limit,
	}
	resp, err := c.session.PostForm(ctx, changeConsumePath, kernel.RequestOptions{Form: form})
	if err != nil {
		return nil, &kernel.OpError{Op: "service.consume.set", Message: "submit failed", Err: err}
	}

	writeResult := &kernel.WriteBackResult{
		PreState:    map[string]string{"installmentFlag": state.InstallmentFlag, "consumeLimit": state.CurrentLimit},
		TargetState: map[string]string{"consumeLimit": limit},
	}

	if readback {
		post, _, err := c.readConsumeProtect(ctx)
		if err != nil {
			return nil, err
		}
		writeResult.PostState = map[string]string{"installmentFlag": post.InstallmentFlag, "consumeLimit": post.CurrentLimit}
		if post.InstallmentFlag != limit {
			return &kernel.OperationResult[kernel.WriteBackResult]{
					Level:   kernel.EvidenceConfirmed,
					Success: false,
					Message: "consume protect readback mismatch",
					Data:    writeResult,
					Raw:     rawCapture(resp),
				}, &kernel.OpError{
					Op:      "service.consume.set",
					Message: fmt.Sprintf("installmentFlag expected=%q got=%q", limit, post.InstallmentFlag),
					Err:     kernel.ErrReadBackMismatch,
					ProblemDetails: kernel.StateComparisonProblemDetails{
						Field:    "installmentFlag",
						Expected: limit,
						Actual:   post.InstallmentFlag,
					},
				}
		}
	}

	if restore {
		restoreForm := map[string]string{"csrftoken": state.CSRFTOKEN, "consumeLimit": state.InstallmentFlag}
		if _, err := c.session.PostForm(ctx, changeConsumePath, kernel.RequestOptions{Form: restoreForm}); err != nil {
			return nil, &kernel.OpError{Op: "service.consume.restore", Message: "restore submit failed", Err: err}
		}
		restored, _, err := c.readConsumeProtect(ctx)
		if err != nil {
			return nil, err
		}
		writeResult.RestoredState = map[string]string{"installmentFlag": restored.InstallmentFlag, "consumeLimit": restored.CurrentLimit}
		if restored.InstallmentFlag != state.InstallmentFlag {
			return &kernel.OperationResult[kernel.WriteBackResult]{
					Level:   kernel.EvidenceConfirmed,
					Success: false,
					Message: "consume protect restore failed",
					Data:    writeResult,
					Raw:     rawCapture(resp),
				}, &kernel.OpError{
					Op:      "service.consume.restore",
					Message: fmt.Sprintf("installmentFlag expected=%q got=%q", state.InstallmentFlag, restored.InstallmentFlag),
					Err:     kernel.ErrRestoreFailed,
					ProblemDetails: kernel.StateComparisonProblemDetails{
						Field:    "installmentFlag",
						Expected: state.InstallmentFlag,
						Actual:   restored.InstallmentFlag,
					},
				}
		}
	}

	return &kernel.OperationResult[kernel.WriteBackResult]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: "consume protect updated",
		Data:    writeResult,
		Raw:     rawCapture(resp),
	}, nil
}

// GetMacList loads the MAC registry JSON list.
func (c *Client) GetMacList(ctx context.Context) (*kernel.OperationResult[kernel.MacListResult], error) {
	if err := c.ensureSession("service.mac.list"); err != nil {
		return nil, err
	}
	preflight, err := c.session.Get(ctx, "/Self/service/myMac", kernel.RequestOptions{})
	if err != nil {
		return nil, &kernel.OpError{Op: "service.mac.list", Message: "myMac preflight failed", Err: err}
	}
	if looksLikeLoginPage(preflight.Body) {
		return nil, &kernel.OpError{Op: "service.mac.list", Message: "myMac returned login page", Err: kernel.ErrAuth}
	}
	resp, err := c.session.Get(ctx, macListPath, kernel.RequestOptions{Query: map[string]string{
		"pageSize":   "10",
		"pageNumber": "1",
		"sortName":   "2",
		"sortOrder":  "DESC",
		"_":          strconv.FormatInt(time.Now().UnixMilli(), 10),
	}})
	if err != nil {
		return nil, &kernel.OpError{Op: "service.mac.list", Message: "request failed", Err: err}
	}
	if strings.TrimSpace(string(resp.Body)) == "" {
		data := &kernel.MacListResult{Total: 0, Rows: []map[string]interface{}{}}
		return &kernel.OperationResult[kernel.MacListResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: true,
			Message: "mac list endpoint returned empty body; treating as zero rows",
			Data:    data,
			Raw:     rawCapture(resp),
		}, nil
	}

	var payload struct {
		Total interface{}              `json:"total"`
		Rows  []map[string]interface{} `json:"rows"`
	}
	if err := parseJSON(resp.Body, &payload); err != nil {
		return nil, &kernel.OpError{Op: "service.mac.list", Message: "parse json failed", Err: err}
	}

	total, _ := strconv.Atoi(toString(payload.Total))
	data := &kernel.MacListResult{Total: total, Rows: payload.Rows}
	return &kernel.OperationResult[kernel.MacListResult]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: fmt.Sprintf("loaded %d mac rows", len(payload.Rows)),
		Data:    data,
		Raw:     rawCapture(resp),
	}, nil
}

func (c *Client) getOperatorState(ctx context.Context) (string, map[string]string, *kernel.SessionResponse, error) {
	doc, resp, err := c.readDocument(ctx, operatorIDPath, kernel.RequestOptions{}, "service.binding.state")
	if err != nil {
		return "", nil, nil, err
	}
	if looksLikeLoginPage(resp.Body) {
		return "", nil, resp, &kernel.OpError{Op: "service.binding.state", Message: "operatorId returned login page", Err: kernel.ErrAuth}
	}
	token, state := parseOperatorState(doc)
	if token == "" {
		return "", nil, resp, &kernel.OpError{Op: "service.binding.state", Message: "missing csrftoken", Err: kernel.ErrTokenExpired}
	}
	return token, state, resp, nil
}

func (c *Client) readConsumeProtect(ctx context.Context) (*kernel.ConsumeProtectState, *kernel.SessionResponse, error) {
	doc, resp, err := c.readDocument(ctx, consumeProtectPath, kernel.RequestOptions{}, "service.consume.state")
	if err != nil {
		return nil, nil, err
	}
	if looksLikeLoginPage(resp.Body) {
		return nil, resp, &kernel.OpError{Op: "service.consume.state", Message: "consumeProtect returned login page", Err: kernel.ErrAuth}
	}

	state := parseConsumeProtectState(doc, string(resp.Body))
	if state.CSRFTOKEN == "" {
		return nil, resp, &kernel.OpError{Op: "service.consume.state", Message: "missing csrftoken", Err: kernel.ErrTokenExpired}
	}
	return state, resp, nil
}
