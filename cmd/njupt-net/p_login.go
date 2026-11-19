package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hicancan/njupt-net/v4/p"
)

type loginReadback struct {
	*p.OperationResult
	AccountVerified  bool `json:"account_verified"`
	OperatorVerified bool `json:"operator_verified"`
}

// A password-authenticated Self session proves who owns a matching terminal.
// The operator remains unverified if the portal omits it.
func loginWithSelfReadback(ctx context.Context, owner *commandContext, portal *p.Portal, alias string, value credential, operator string) (any, error) {
	result, loginErr := portal.Login(ctx, value.Account, value.Password, operator)
	if result == nil || result.Outcome != p.Accepted {
		return result, loginErr
	}
	readback := loginReadback{OperationResult: result, AccountVerified: result.Verified, OperatorVerified: result.Verified}
	if result.Verified {
		return &readback, loginErr
	}
	ctx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	if result.Status == nil || result.Status.Terminal == nil {
		current, err := portal.Status(ctx)
		if err != nil {
			return &readback, errors.Join(loginErr, err)
		}
		result.Status = current
		if current.Terminal == nil {
			return &readback, loginErr
		}
	}
	terminal := result.Status.Terminal
	if !strings.ContainsAny(terminal.Account, "*＊") {
		return &readback, loginErr
	}
	_, reportedOperator, hasOperator := strings.Cut(terminal.Account, "@")
	if hasOperator && reportedOperator != operator {
		return &readback, loginErr
	}
	self, err := owner.self(ctx, alias, value, "")
	if err != nil {
		return &readback, errors.Join(loginErr, fmt.Errorf("Self ownership verification: %w", err))
	}
	macFormat := strings.NewReplacer(":", "", "-", "")
	for {
		connections, err := self.Online(ctx)
		if err != nil {
			owner.forgetExpired(alias, err)
			return &readback, errors.Join(loginErr, fmt.Errorf("Self ownership verification: %w", err))
		}
		matches := 0
		for _, connection := range connections {
			mac := macFormat.Replace(connection.MAC)
			if connection.SessionID != "" && connection.IP == terminal.IP && strings.EqualFold(mac, terminal.MAC) {
				matches++
			}
		}
		if matches == 1 {
			break
		}
		if matches > 1 {
			return &readback, errors.Join(loginErr, fmt.Errorf("Self reports multiple matching connections"))
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return &readback, errors.Join(loginErr, fmt.Errorf("Self did not report a matching connection: %w", ctx.Err()))
		case <-timer.C:
		}
	}
	current, err := portal.Status(ctx)
	if err != nil {
		return &readback, errors.Join(loginErr, fmt.Errorf("confirm portal state after the Self lookup: %w", err))
	}
	if !current.Online || current.Terminal == nil || *current.Terminal != *terminal {
		return &readback, errors.Join(loginErr, fmt.Errorf("portal terminal changed during Self ownership verification"))
	}
	readback.AccountVerified = true
	readback.OperatorVerified = hasOperator
	readback.Verified = readback.OperatorVerified
	return &readback, nil
}
