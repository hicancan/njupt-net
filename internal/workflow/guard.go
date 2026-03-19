package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hicancan/njupt-net-cli/internal/config"
	"github.com/hicancan/njupt-net-cli/internal/httpx"
	"github.com/hicancan/njupt-net-cli/internal/kernel"
	"github.com/hicancan/njupt-net-cli/internal/portal"
	"github.com/hicancan/njupt-net-cli/internal/selfservice"
)

// GuardProber provides connectivity and local IPv4 detection to the workflow layer.
type GuardProber interface {
	CheckConnectivity(ctx context.Context) (bool, string)
	DetectLocalIPv4(ctx context.Context) (string, error)
}

// GuardSelfClient captures the Self operations the guard workflow needs.
type GuardSelfClient interface {
	Login(ctx context.Context, username, password string) (*kernel.OperationResult[kernel.SelfLoginResult], error)
	GetOperatorBinding(ctx context.Context) (*kernel.OperationResult[kernel.OperatorBinding], error)
	BindOperator(ctx context.Context, target map[string]string, readback, restore bool) (*kernel.OperationResult[kernel.WriteBackResult], error)
}

// GuardPortalClient captures the Portal operation the guard workflow needs.
type GuardPortalClient interface {
	Login802(ctx context.Context, account, password, ip, isp string) (*kernel.OperationResult[kernel.Portal802Response], error)
}

// GuardClientFactory creates fresh clients for each workflow operation.
type GuardClientFactory interface {
	NewSelf() (GuardSelfClient, error)
	NewPortal() (GuardPortalClient, error)
}

// RealGuardClientFactory creates concrete Self/Portal clients.
type RealGuardClientFactory struct {
	SelfBaseURL           string
	PortalBaseURL         string
	PortalFallbackBaseURL string
	SelfTimeout           time.Duration
	PortalTimeout         time.Duration
	InsecureTLS           bool
}

// NewSelf creates a fresh concrete Self client.
func (f RealGuardClientFactory) NewSelf() (GuardSelfClient, error) {
	session, err := httpx.NewSessionClient(httpx.Options{
		BaseURL:     f.SelfBaseURL,
		Timeout:     f.SelfTimeout,
		InsecureTLS: f.InsecureTLS,
	})
	if err != nil {
		return nil, err
	}
	return selfservice.NewClient(session), nil
}

// NewPortal creates a fresh concrete Portal client.
func (f RealGuardClientFactory) NewPortal() (GuardPortalClient, error) {
	session, err := httpx.NewSessionClient(httpx.Options{
		BaseURL:     f.PortalBaseURL,
		Timeout:     f.PortalTimeout,
		InsecureTLS: f.InsecureTLS,
	})
	if err != nil {
		return nil, err
	}
	return portal.NewClient(session, f.PortalBaseURL, f.PortalFallbackBaseURL), nil
}

// BindingRepairResult summarizes the non-secret repair outcome.
type BindingRepairResult struct {
	TargetProfile string `json:"targetProfile"`
	HolderProfile string `json:"holderProfile,omitempty"`
	Action        string `json:"action"`
}

// EnsureOnlineResult summarizes one aggressive ensure-online chain.
type EnsureOnlineResult struct {
	DesiredProfile         string                    `json:"desiredProfile"`
	InitialIP              string                    `json:"initialIp,omitempty"`
	RetryIP                string                    `json:"retryIp,omitempty"`
	FirstPortalLoginOK     bool                      `json:"firstPortalLoginOk"`
	FirstPortalLoginMsg    string                    `json:"firstPortalLoginMessage,omitempty"`
	SecondPortalLoginOK    bool                      `json:"secondPortalLoginOk"`
	SecondPortalLoginMsg   string                    `json:"secondPortalLoginMessage,omitempty"`
	BindingRepairAttempted bool                      `json:"bindingRepairAttempted"`
	BindingRepairOK        bool                      `json:"bindingRepairOk"`
	BindingRepairMessage   string                    `json:"bindingRepairMessage,omitempty"`
	BindingRepair          *BindingRepairResult      `json:"bindingRepair,omitempty"`
	PortalPayload          *kernel.Portal802Response `json:"portalPayload,omitempty"`
	InternetOK             bool                      `json:"internetOk"`
	InternetMessage        string                    `json:"internetMessage,omitempty"`
	RecoveryStep           string                    `json:"recoveryStep"`
}

// GuardCycleInput is the runtime-to-workflow control surface for one cycle.
type GuardCycleInput struct {
	DesiredProfile    string `json:"desiredProfile"`
	ScheduleWindow    string `json:"scheduleWindow"`
	ForceSwitch       bool   `json:"forceSwitch"`
	ForceBindingCheck bool   `json:"forceBindingCheck"`
}

// GuardCycleResult is the typed, non-secret output for one guard cycle.
type GuardCycleResult struct {
	DesiredProfile     string               `json:"desiredProfile"`
	ScheduleWindow     string               `json:"scheduleWindow"`
	ForceSwitch        bool                 `json:"forceSwitch"`
	ForceBindingCheck  bool                 `json:"forceBindingCheck"`
	BindingOK          bool                 `json:"bindingOk"`
	BindingMessage     string               `json:"bindingMessage,omitempty"`
	InitialInternetOK  bool                 `json:"initialInternetOk"`
	InitialInternetMsg string               `json:"initialInternetMessage,omitempty"`
	InternetOK         bool                 `json:"internetOk"`
	InternetMessage    string               `json:"internetMessage,omitempty"`
	PortalLoginOK      bool                 `json:"portalLoginOk"`
	PortalLoginMessage string               `json:"portalLoginMessage,omitempty"`
	RecoveryStep       string               `json:"recoveryStep"`
	LocalIP            string               `json:"localIp,omitempty"`
	RetryLocalIP       string               `json:"retryLocalIp,omitempty"`
	BindingRepair      *BindingRepairResult `json:"bindingRepair,omitempty"`
	EnsureOnline       *EnsureOnlineResult  `json:"ensureOnline,omitempty"`
}

// GuardEnvironment contains the dependencies needed for guard workflows.
type GuardEnvironment struct {
	Accounts  map[string]config.AccountConfig
	Broadband config.BroadbandConfig
	PortalISP string
	Factory   GuardClientFactory
	Prober    GuardProber
}

// RepairBinding ensures the desired profile owns the configured mobile broadband credentials.
func RepairBinding(ctx context.Context, env GuardEnvironment, targetProfile string) (*kernel.OperationResult[BindingRepairResult], error) {
	targetAccount, ok := env.Accounts[targetProfile]
	if !ok {
		return nil, &kernel.OpError{Op: "workflow.guard.repairBinding", Message: fmt.Sprintf("target profile %q not found", targetProfile), Err: kernel.ErrInvalidConfig}
	}
	targetClient, err := env.Factory.NewSelf()
	if err != nil {
		return nil, &kernel.OpError{Op: "workflow.guard.repairBinding", Message: "create target self client failed", Err: err}
	}
	if _, err := targetClient.Login(ctx, targetAccount.Username, targetAccount.Password); err != nil {
		return nil, &kernel.OpError{Op: "workflow.guard.repairBinding", Message: "target self login failed", Err: err}
	}
	targetBinding, err := targetClient.GetOperatorBinding(ctx)
	if err != nil {
		return nil, &kernel.OpError{Op: "workflow.guard.repairBinding", Message: "target binding read failed", Err: err}
	}
	if targetBinding.Data == nil {
		return nil, &kernel.OpError{Op: "workflow.guard.repairBinding", Message: "target binding state missing", Err: kernel.ErrBusinessFailed}
	}
	if targetBinding.Data.MobileAccount == env.Broadband.Account {
		result := &BindingRepairResult{
			TargetProfile: targetProfile,
			Action:        "already-correct",
		}
		message := "target binding already correct"
		if targetBinding.Data.MobilePassword != "" && targetBinding.Data.MobilePassword != env.Broadband.Password {
			message = "target binding already owns account but password readback differs"
		}
		return &kernel.OperationResult[BindingRepairResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: true,
			Message: message,
			Data:    result,
		}, nil
	}

	holderProfile := ""
	for profile, account := range env.Accounts {
		if profile == targetProfile {
			continue
		}
		holderClient, err := env.Factory.NewSelf()
		if err != nil {
			continue
		}
		if _, err := holderClient.Login(ctx, account.Username, account.Password); err != nil {
			continue
		}
		holderBinding, err := holderClient.GetOperatorBinding(ctx)
		if err != nil || holderBinding.Data == nil {
			continue
		}
		if holderBinding.Data.MobileAccount != env.Broadband.Account {
			continue
		}
		holderProfile = profile
		if _, err := holderClient.BindOperator(ctx, map[string]string{
			"FLDEXTRA3": "",
			"FLDEXTRA4": "",
		}, true, false); err != nil {
			return &kernel.OperationResult[BindingRepairResult]{
				Level:   kernel.EvidenceConfirmed,
				Success: false,
				Message: fmt.Sprintf("failed to clear holder %s", profile),
				Data: &BindingRepairResult{
					TargetProfile: targetProfile,
					HolderProfile: profile,
					Action:        "holder-clear-failed",
				},
			}, err
		}
		break
	}

	if _, err := targetClient.BindOperator(ctx, map[string]string{
		"FLDEXTRA3": env.Broadband.Account,
		"FLDEXTRA4": env.Broadband.Password,
	}, true, false); err != nil {
		return &kernel.OperationResult[BindingRepairResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: fmt.Sprintf("failed to bind target %s", targetProfile),
			Data: &BindingRepairResult{
				TargetProfile: targetProfile,
				HolderProfile: holderProfile,
				Action:        "target-bind-failed",
			},
		}, err
	}

	action := "attached"
	message := fmt.Sprintf("binding attached to %s", targetProfile)
	if holderProfile != "" {
		action = "moved"
		message = fmt.Sprintf("binding moved from %s to %s", holderProfile, targetProfile)
	}
	return &kernel.OperationResult[BindingRepairResult]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: message,
		Data: &BindingRepairResult{
			TargetProfile: targetProfile,
			HolderProfile: holderProfile,
			Action:        action,
		},
	}, nil
}

// EnsureOnline aggressively restores connectivity for the desired profile.
func EnsureOnline(ctx context.Context, env GuardEnvironment, targetProfile string, forcePortalLogin bool) (*kernel.OperationResult[EnsureOnlineResult], error) {
	account, ok := env.Accounts[targetProfile]
	if !ok {
		return nil, &kernel.OpError{Op: "workflow.guard.ensureOnline", Message: fmt.Sprintf("target profile %q not found", targetProfile), Err: kernel.ErrInvalidConfig}
	}
	result := &EnsureOnlineResult{
		DesiredProfile: targetProfile,
		RecoveryStep:   "no-ip",
	}
	_ = forcePortalLogin

	ip, err := env.Prober.DetectLocalIPv4(ctx)
	if err != nil || strings.TrimSpace(ip) == "" {
		return &kernel.OperationResult[EnsureOnlineResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: "unable to detect local IPv4",
			Data:    result,
		}, &kernel.OpError{Op: "workflow.guard.ensureOnline", Message: "unable to detect local IPv4", Err: err}
	}
	result.InitialIP = ip

	portalClient, err := env.Factory.NewPortal()
	if err != nil {
		return nil, &kernel.OpError{Op: "workflow.guard.ensureOnline", Message: "create portal client failed", Err: err}
	}
	first, firstErr := portalClient.Login802(ctx, account.Username, account.Password, ip, env.PortalISP)
	result.FirstPortalLoginOK = firstErr == nil
	if first != nil {
		result.FirstPortalLoginMsg = first.Message
		result.PortalPayload = first.Data
	}
	if firstErr == nil {
		internetOK, internetMessage := env.Prober.CheckConnectivity(ctx)
		result.InternetOK = internetOK
		result.InternetMessage = internetMessage
		if internetOK {
			result.RecoveryStep = "portal-login"
			return &kernel.OperationResult[EnsureOnlineResult]{
				Level:   kernel.EvidenceConfirmed,
				Success: true,
				Message: "portal login restored connectivity",
				Data:    result,
			}, nil
		}
	}

	repair, repairErr := RepairBinding(ctx, env, targetProfile)
	result.BindingRepairAttempted = true
	if repair != nil {
		result.BindingRepairOK = repair.Success
		result.BindingRepairMessage = repair.Message
		result.BindingRepair = repair.Data
	}
	if repairErr != nil {
		result.RecoveryStep = "binding-repair-failed"
		return &kernel.OperationResult[EnsureOnlineResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: "binding repair failed before retry login",
			Data:    result,
		}, repairErr
	}

	retryIP, err := env.Prober.DetectLocalIPv4(ctx)
	if err != nil || strings.TrimSpace(retryIP) == "" {
		retryIP = ip
	}
	result.RetryIP = retryIP
	second, secondErr := portalClient.Login802(ctx, account.Username, account.Password, retryIP, env.PortalISP)
	result.SecondPortalLoginOK = secondErr == nil
	if second != nil {
		result.SecondPortalLoginMsg = second.Message
		result.PortalPayload = second.Data
	}
	internetOK, internetMessage := env.Prober.CheckConnectivity(ctx)
	result.InternetOK = internetOK
	result.InternetMessage = internetMessage
	result.RecoveryStep = "binding-repair-then-portal-login"
	if internetOK {
		return &kernel.OperationResult[EnsureOnlineResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: true,
			Message: "binding repair and portal login restored connectivity",
			Data:    result,
		}, nil
	}
	return &kernel.OperationResult[EnsureOnlineResult]{
		Level:   kernel.EvidenceConfirmed,
		Success: false,
		Message: "connectivity still unavailable after portal retry",
		Data:    result,
	}, secondErr
}

// GuardCycle executes one non-secret guard cycle around schedule, probe, repair, and portal recovery.
func GuardCycle(ctx context.Context, env GuardEnvironment, input GuardCycleInput) (*GuardCycleResult, error) {
	result := &GuardCycleResult{
		DesiredProfile:     input.DesiredProfile,
		ScheduleWindow:     input.ScheduleWindow,
		ForceSwitch:        input.ForceSwitch,
		ForceBindingCheck:  input.ForceBindingCheck,
		BindingOK:          true,
		BindingMessage:     "binding check skipped",
		PortalLoginOK:      true,
		PortalLoginMessage: "portal login not needed",
		RecoveryStep:       "healthy",
	}

	if input.ForceSwitch {
		repair, err := RepairBinding(ctx, env, input.DesiredProfile)
		if repair != nil {
			result.BindingOK = repair.Success
			result.BindingMessage = repair.Message
			result.BindingRepair = repair.Data
		}
		if err != nil {
			result.BindingOK = false
			result.PortalLoginOK = false
			result.PortalLoginMessage = "portal login skipped because switch binding repair failed"
			result.RecoveryStep = "switch-binding-repair-failed"
			return result, err
		}

		ensure, err := EnsureOnline(ctx, env, input.DesiredProfile, true)
		if ensure != nil && ensure.Data != nil {
			applyEnsureOnline(result, ensure.Data)
		}
		if err != nil {
			return result, err
		}
		return result, nil
	}

	if input.ForceBindingCheck {
		repair, err := RepairBinding(ctx, env, input.DesiredProfile)
		if repair != nil {
			result.BindingOK = repair.Success
			result.BindingMessage = repair.Message
			result.BindingRepair = repair.Data
		}
		if err != nil {
			result.BindingOK = false
			result.RecoveryStep = "degraded-binding-repair"
		}
	}

	internetOK, internetMessage := env.Prober.CheckConnectivity(ctx)
	result.InitialInternetOK = internetOK
	result.InitialInternetMsg = internetMessage
	result.InternetOK = internetOK
	result.InternetMessage = internetMessage
	if internetOK {
		return result, nil
	}

	ensure, err := EnsureOnline(ctx, env, input.DesiredProfile, false)
	if ensure != nil && ensure.Data != nil {
		applyEnsureOnline(result, ensure.Data)
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func applyEnsureOnline(dst *GuardCycleResult, src *EnsureOnlineResult) {
	dst.EnsureOnline = src
	dst.LocalIP = src.InitialIP
	dst.RetryLocalIP = src.RetryIP
	dst.PortalLoginOK = src.InternetOK
	dst.InternetOK = src.InternetOK
	dst.InternetMessage = src.InternetMessage
	dst.PortalLoginMessage = portalMessageFromEnsure(src)
	dst.RecoveryStep = src.RecoveryStep
	if src.BindingRepairAttempted {
		dst.BindingOK = src.BindingRepairOK
		dst.BindingMessage = src.BindingRepairMessage
		if src.BindingRepair != nil {
			dst.BindingRepair = src.BindingRepair
		}
	}
}

func portalMessageFromEnsure(src *EnsureOnlineResult) string {
	switch {
	case src.SecondPortalLoginMsg != "":
		return src.SecondPortalLoginMsg
	case src.FirstPortalLoginMsg != "":
		return src.FirstPortalLoginMsg
	case src.InternetMessage != "":
		return src.InternetMessage
	default:
		return "portal login not needed"
	}
}
