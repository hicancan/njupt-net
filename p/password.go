package p

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/hicancan/njupt-net/v4/network"
)

func (p *Portal) Captcha(ctx context.Context) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.captchaReady = false
	nonce, err := requestNonce()
	if err != nil {
		return nil, err
	}
	data, _, header, err := network.Request(ctx, p.client, http.MethodGet, p.api+"captcha", url.Values{"randomNum": {nonce}})
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || !strings.HasPrefix(strings.ToLower(header.Get("Content-Type")), "image/") {
		return nil, fmt.Errorf("portal captcha did not return an image")
	}
	p.captchaReady = true
	return data, nil
}

// CheckCaptcha verifies one answer in the same Portal cookie session that issued the image.
func (p *Portal) CheckCaptcha(ctx context.Context, answer string) (*OperationResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return nil, fmt.Errorf("captcha answer is required")
	}
	if !p.captchaReady {
		return nil, fmt.Errorf("fetch a new captcha in this portal session before checking an answer")
	}
	p.captchaReady = false
	raw, err := p.call(ctx, "captcha/check", url.Values{"captcha": {answer}})
	if err != nil {
		return unknown("captcha check", err)
	}
	result, err := operationResult(raw, "captcha check")
	if err == nil {
		result.Verified = true
	}
	return result, err
}

// ChangePassword reports server acceptance. Verified remains false because
// this operation does not authenticate with the new password.
func (p *Portal) ChangePassword(ctx context.Context, account, password, newPassword, captcha string) (*OperationResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := validateCredential(account, password); err != nil {
		return nil, err
	}
	if newPassword == "" || captcha == "" {
		return nil, fmt.Errorf("new password and captcha are required")
	}
	if password == newPassword {
		return nil, fmt.Errorf("new password must differ from the current password")
	}
	if err := p.ensure(ctx); err != nil {
		return nil, err
	}
	p.captchaReady = false
	raw, err := p.call(ctx, "change_pass", url.Values{"user_account": {base64Text(account)}, "user_old_password": {base64Text(password)}, "user_new_password": {base64Text(newPassword)}, "registerMode": {scalar(p.settings["register_mode"])}, "captcha": {captcha}})
	if err != nil {
		return unknown("password change", err)
	}
	return operationResult(raw, "password change")
}
