package workflow

import (
	"context"
	"fmt"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

// MigrationSelfClient captures the minimal Self operations needed by the migration workflow.
type MigrationSelfClient interface {
	Login(ctx context.Context, username, password string) (*kernel.OperationResult[kernel.SelfLoginResult], error)
	BindOperator(ctx context.Context, target map[string]string, readback, restore bool) (*kernel.OperationResult[kernel.WriteBackResult], error)
}

// MigrationFactory creates fresh Self clients for each workflow stage.
type MigrationFactory interface {
	NewSelf() (MigrationSelfClient, error)
}

// MigrationInput is the complete migration request.
type MigrationInput struct {
	From         Credentials
	To           Credentials
	TargetFields map[string]string
}

// MigrateResult captures the source clear and target bind evidence chain.
type MigrateResult struct {
	SourceClear *kernel.OperationResult[kernel.WriteBackResult] `json:"sourceClear,omitempty"`
	TargetBind  *kernel.OperationResult[kernel.WriteBackResult] `json:"targetBind,omitempty"`
}

// MigrateBroadband performs a guarded multi-step migration workflow.
func MigrateBroadband(ctx context.Context, factory MigrationFactory, input MigrationInput) (*kernel.OperationResult[MigrateResult], error) {
	if factory == nil {
		return nil, &kernel.OpError{Op: "workflow.migrate", Message: "migration factory is required", Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "migrationFactory", Hint: "construct workflows through app context"}}
	}
	if input.From.Username == "" || input.From.Password == "" || input.To.Username == "" || input.To.Password == "" {
		return nil, &kernel.OpError{Op: "workflow.migrate", Message: "from/to credentials are required", Err: kernel.ErrBusinessFailed}
	}
	if len(input.TargetFields) == 0 {
		return nil, &kernel.OpError{Op: "workflow.migrate", Message: "target fields are required", Err: kernel.ErrBusinessFailed}
	}

	srcClient, err := factory.NewSelf()
	if err != nil {
		return nil, &kernel.OpError{Op: "workflow.migrate", Message: "create source session failed", Err: err}
	}
	if _, err := srcClient.Login(ctx, input.From.Username, input.From.Password); err != nil {
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
			Level:    kernel.EvidenceConfirmed,
			Success:  false,
			Message:  "source clear failed",
			Data:     &MigrateResult{SourceClear: sourceClear},
			Problems: kernel.ProblemsFromError(err),
		}, err
	}

	dstClient, err := factory.NewSelf()
	if err != nil {
		return nil, &kernel.OpError{Op: "workflow.migrate", Message: "create target session failed", Err: err}
	}
	if _, err := dstClient.Login(ctx, input.To.Username, input.To.Password); err != nil {
		return nil, &kernel.OpError{Op: "workflow.migrate", Message: "target login failed", Err: err}
	}

	targetBind, err := dstClient.BindOperator(ctx, input.TargetFields, true, false)
	if err != nil {
		return &kernel.OperationResult[MigrateResult]{
			Level:    kernel.EvidenceConfirmed,
			Success:  false,
			Message:  "target bind failed",
			Data:     &MigrateResult{SourceClear: sourceClear, TargetBind: targetBind},
			Problems: kernel.ProblemsFromError(err),
		}, err
	}

	return &kernel.OperationResult[MigrateResult]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: fmt.Sprintf("migration completed from %s to %s", input.From.Username, input.To.Username),
		Data: &MigrateResult{
			SourceClear: sourceClear,
			TargetBind:  targetBind,
		},
	}, nil
}
