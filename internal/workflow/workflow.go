package workflow

import (
	"context"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
	"github.com/hicancan/njupt-net-cli/internal/selfservice"
)

// DoctorResult summarizes a full login/status health chain.
type DoctorResult struct {
	Login  *kernel.OperationResult[kernel.SelfLoginResult] `json:"login,omitempty"`
	Status *kernel.OperationResult[kernel.SelfStatus]      `json:"status,omitempty"`
}

// SelfDoctor executes the canonical Self diagnosis chain.
func SelfDoctor(ctx context.Context, client *selfservice.Client, username, password string) (*kernel.OperationResult[DoctorResult], error) {
	loginResult, err := client.Login(ctx, username, password)
	if err != nil {
		return &kernel.OperationResult[DoctorResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: "self doctor login stage failed",
			Data:    &DoctorResult{Login: loginResult},
		}, err
	}

	statusResult, err := client.Status(ctx)
	if err != nil {
		return &kernel.OperationResult[DoctorResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: "self doctor status stage failed",
			Data:    &DoctorResult{Login: loginResult, Status: statusResult},
		}, err
	}

	data := &DoctorResult{
		Login:  loginResult,
		Status: statusResult,
	}
	return &kernel.OperationResult[DoctorResult]{
		Level:   kernel.EvidenceConfirmed,
		Success: loginResult.Success && statusResult.Success && statusResult.Data != nil && statusResult.Data.LoggedIn,
		Message: "self doctor completed",
		Data:    data,
	}, nil
}
