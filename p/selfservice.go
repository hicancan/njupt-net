package p

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

type BridgeResult struct {
	Outcome Outcome `json:"outcome"`
	Type    int     `json:"type"`
	URL     string  `json:"url"`
	Message string  `json:"message,omitempty"`
}

// SelfURL returns the authenticated bridge URL without following it or creating
// a management session. The caller chooses whether to visit that destination.
func (p *Portal) SelfURL(ctx context.Context, account, password string, selfType int) (*BridgeResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if selfType < 0 || selfType > 2 {
		return nil, fmt.Errorf("self type must be 0, 1 or 2")
	}
	if err := validateCredential(account, password); err != nil {
		return nil, err
	}
	raw, err := p.call(ctx, "self", url.Values{"self_type": {strconv.Itoa(selfType)}, "user_account": {account}, "user_password": {password}})
	if err != nil {
		return nil, err
	}
	result, err := operationResult(raw, "self bridge")
	if err != nil {
		return &BridgeResult{Outcome: result.Outcome, Type: selfType, Message: result.Message}, err
	}
	target, err := url.Parse(result.SelfURL)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return nil, fmt.Errorf("portal self bridge did not return an absolute HTTP URL")
	}
	return &BridgeResult{Outcome: result.Outcome, Type: selfType, URL: target.String(), Message: result.Message}, nil
}
