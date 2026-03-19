package selfservice

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

// Login executes the SSOT-authenticated login chain and verifies protected readability.
func (c *Client) Login(ctx context.Context, account, password string) (*kernel.OperationResult[kernel.SelfLoginResult], error) {
	if err := c.ensureSession("self.login"); err != nil {
		return nil, err
	}

	result := &kernel.SelfLoginResult{}

	doc, loginResp, err := c.readDocument(ctx, loginPath, kernel.RequestOptions{}, "self.login.preflight")
	if err != nil {
		return nil, err
	}
	checkcode := extractInputValue(doc, "checkcode")
	result.CheckcodeFetched = checkcode != ""
	if checkcode == "" {
		return &kernel.OperationResult[kernel.SelfLoginResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: "missing checkcode token",
			Data:    result,
			Raw:     rawCapture(loginResp),
		}, &kernel.OpError{Op: "self.login.preflight", Message: "missing checkcode token", Err: kernel.ErrNeedFreshLoginPage}
	}

	if _, err := c.session.Get(ctx, randomCodePath, kernel.RequestOptions{
		Query: map[string]string{"t": fmt.Sprintf("%d", time.Now().UnixMilli())},
	}); err != nil {
		return nil, &kernel.OpError{Op: "self.login.randomCode", Message: "randomCode request failed", Err: err}
	}
	result.RandomCodeCalled = true

	verifyResp, err := c.session.PostForm(ctx, verifyPath, kernel.RequestOptions{
		Form: map[string]string{
			"account":   account,
			"password":  password,
			"checkcode": checkcode,
			"code":      "",
			"foo":       account,
			"bar":       password,
		},
	})
	if err != nil {
		return nil, &kernel.OpError{Op: "self.login.verify", Message: "verify request failed", Err: err}
	}

	result.VerifyStatus = verifyResp.StatusCode
	result.VerifyLocation = verifyResp.FinalURL

	status, err := c.Status(ctx)
	if err == nil && status != nil && status.Data != nil {
		result.DashboardReadable = status.Data.DashboardReadable
		result.SessionAlive = status.Data.LoggedIn
	}

	opResult := &kernel.OperationResult[kernel.SelfLoginResult]{
		Level:   kernel.EvidenceConfirmed,
		Success: result.DashboardReadable && result.SessionAlive,
		Message: "self login succeeded",
		Data:    result,
		Raw:     rawCapture(verifyResp),
	}
	if opResult.Success {
		return opResult, nil
	}

	message := "login failed"
	if strings.Contains(verifyResp.FinalURL, "/Self/login") || looksLikeLoginPage(verifyResp.Body) {
		message = extractLoginErrorMessage(verifyResp.Body)
	}
	opResult.Message = message
	return opResult, &kernel.OpError{Op: "self.login", Message: message, Err: kernel.ErrAuth}
}

// Logout invalidates the current session and verifies protected pages are no longer readable.
func (c *Client) Logout(ctx context.Context) (*kernel.OperationResult[kernel.SelfStatus], error) {
	if err := c.ensureSession("self.logout"); err != nil {
		return nil, err
	}
	resp, err := c.session.Get(ctx, logoutPath, kernel.RequestOptions{})
	if err != nil {
		return nil, &kernel.OpError{Op: "self.logout", Message: "logout request failed", Err: err}
	}

	status, statusErr := c.Status(ctx)
	if statusErr == nil && status != nil && status.Data != nil && !status.Data.LoggedIn {
		status.Message = "self logout succeeded"
		status.Raw = rawCapture(resp)
		return status, nil
	}

	result := &kernel.OperationResult[kernel.SelfStatus]{
		Level:   kernel.EvidenceConfirmed,
		Success: false,
		Message: "logout could not be verified",
		Raw:     rawCapture(resp),
	}
	if status != nil {
		result.Data = status.Data
	}
	return result, &kernel.OpError{Op: "self.logout", Message: result.Message, Err: kernel.ErrBusinessFailed}
}

// Status checks protected readability without mutating server state.
func (c *Client) Status(ctx context.Context) (*kernel.OperationResult[kernel.SelfStatus], error) {
	if err := c.ensureSession("self.status"); err != nil {
		return nil, err
	}

	dashboardResp, err := c.session.Get(ctx, dashboardPath, kernel.RequestOptions{})
	if err != nil {
		return nil, &kernel.OpError{Op: "self.status.dashboard", Message: "dashboard request failed", Err: err}
	}
	serviceResp, err := c.session.Get(ctx, servicePath, kernel.RequestOptions{})
	if err != nil {
		return nil, &kernel.OpError{Op: "self.status.service", Message: "service request failed", Err: err}
	}

	status := &kernel.SelfStatus{
		DashboardReadable: !looksLikeLoginPage(dashboardResp.Body),
		ServiceReadable:   !looksLikeLoginPage(serviceResp.Body),
	}
	status.LoggedIn = status.DashboardReadable || status.ServiceReadable
	if status.LoggedIn {
		status.Reason = "protected pages readable"
	} else {
		status.Reason = "protected pages redirected to login"
	}

	return &kernel.OperationResult[kernel.SelfStatus]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: status.Reason,
		Data:    status,
		Raw:     rawCapture(dashboardResp),
	}, nil
}
