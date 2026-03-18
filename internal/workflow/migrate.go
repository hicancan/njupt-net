package workflow

import (
	"context"
	"fmt"

	"github.com/hicancan/njupt-net-cli/internal/httpx"
	"github.com/hicancan/njupt-net-cli/internal/kernel"
	"github.com/hicancan/njupt-net-cli/internal/selfservice"
)

// MigrateResult captures the source clear and target bind evidence chain.
type MigrateResult struct {
	SourceClear *kernel.OperationResult[kernel.WriteBackResult] `json:"sourceClear,omitempty"`
	TargetBind  *kernel.OperationResult[kernel.WriteBackResult] `json:"targetBind,omitempty"`
}

var newSessionClient = func(selfBaseURL string) (kernel.SessionClient, error) {
	return httpx.NewSessionClient(httpx.Options{BaseURL: selfBaseURL})
}

// MigrateBroadband performs a guarded multi-step migration workflow.
func MigrateBroadband(
	ctx context.Context,
	selfBaseURL string,
	fromUser,
	fromPwd,
	toUser,
	toPwd string,
	targetFields map[string]string,
) (*kernel.OperationResult[MigrateResult], error) {
	if fromUser == "" || fromPwd == "" || toUser == "" || toPwd == "" {
		return nil, &kernel.OpError{Op: "workflow.migrate", Message: "from/to credentials are required", Err: kernel.ErrBusinessFailed}
	}
	if len(targetFields) == 0 {
		return nil, &kernel.OpError{Op: "workflow.migrate", Message: "target fields are required", Err: kernel.ErrBusinessFailed}
	}

	srcSession, err := newSessionClient(selfBaseURL)
	if err != nil {
		return nil, &kernel.OpError{Op: "workflow.migrate", Message: "create source session failed", Err: err}
	}
	srcClient := selfservice.NewClient(srcSession)
	if _, err := srcClient.Login(ctx, fromUser, fromPwd); err != nil {
		return nil, &kernel.OpError{Op: "workflow.migrate", Message: "source login failed", Err: err}
	}

	clearFields := map[string]string{
		"FLDEXTRA1": "",
		"FLDEXTRA2": "",
		"FLDEXTRA3": "",
		"FLDEXTRA4": "",
	}
	sourceClear, err := srcClient.BindOperator(ctx, clearFields, true, false)
	if err != nil {
		return &kernel.OperationResult[MigrateResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: "source clear failed",
			Data:    &MigrateResult{SourceClear: sourceClear},
		}, err
	}

	dstSession, err := newSessionClient(selfBaseURL)
	if err != nil {
		return nil, &kernel.OpError{Op: "workflow.migrate", Message: "create target session failed", Err: err}
	}
	dstClient := selfservice.NewClient(dstSession)
	if _, err := dstClient.Login(ctx, toUser, toPwd); err != nil {
		return nil, &kernel.OpError{Op: "workflow.migrate", Message: "target login failed", Err: err}
	}

	targetBind, err := dstClient.BindOperator(ctx, targetFields, true, false)
	if err != nil {
		return &kernel.OperationResult[MigrateResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: "target bind failed",
			Data:    &MigrateResult{SourceClear: sourceClear, TargetBind: targetBind},
		}, err
	}

	return &kernel.OperationResult[MigrateResult]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: fmt.Sprintf("migration completed from %s to %s", fromUser, toUser),
		Data: &MigrateResult{
			SourceClear: sourceClear,
			TargetBind:  targetBind,
		},
	}, nil
}
