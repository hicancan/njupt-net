package workflow

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

// GuardProber provides connectivity and local IPv4 detection to the workflow layer.
type GuardProber interface {
	CheckConnectivity(ctx context.Context) (bool, string)
	DetectLocalIPv4(ctx context.Context) (LocalIPSelection, error)
}

// GuardSelfClient captures the Self operations the guard workflow needs.
type GuardSelfClient interface {
	Login(ctx context.Context, username, password string) (*kernel.OperationResult[kernel.SelfLoginResult], error)
	GetOnlineList(ctx context.Context) (*kernel.OperationResult[[]kernel.OnlineSession], error)
	ForceOffline(ctx context.Context, sessionID string) (*kernel.OperationResult[map[string]any], error)
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

// Credentials is the minimal username/password pair workflows need.
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// BroadbandCredentials is the minimal broadband account/password pair workflows need.
type BroadbandCredentials struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

// LocalIPSelection captures how the guard chose a local IPv4 for Portal traffic.
type LocalIPSelection struct {
	SelectedIP      string `json:"selectedIp,omitempty"`
	RoutedIP        string `json:"routedIp,omitempty"`
	SelectionReason string `json:"selectionReason,omitempty"`
}

// BindingRepairResult summarizes the non-secret repair outcome.
type BindingRepairResult struct {
	TargetProfile     string                `json:"targetProfile"`
	HolderProfile     string                `json:"holderProfile,omitempty"`
	Action            string                `json:"action"`
	TargetOffline     *OfflineCleanupResult `json:"targetOffline,omitempty"`
	RollbackAttempted bool                  `json:"rollbackAttempted,omitempty"`
	RollbackOK        bool                  `json:"rollbackOk,omitempty"`
	RollbackProfile   string                `json:"rollbackProfile,omitempty"`
	RollbackMessage   string                `json:"rollbackMessage,omitempty"`
}

// OfflineCleanupResult summarizes self-service online-session cleanup.
type OfflineCleanupResult struct {
	Scope        string                        `json:"scope,omitempty"`
	Profiles     []ProfileOfflineCleanupResult `json:"profiles,omitempty"`
	SessionCount int                           `json:"sessionCount"`
	RemovedCount int                           `json:"removedCount"`
	SkippedCount int                           `json:"skippedCount,omitempty"`
	FailedCount  int                           `json:"failedCount"`
	Message      string                        `json:"message,omitempty"`
}

// ProfileOfflineCleanupResult summarizes cleanup for one Self account profile.
type ProfileOfflineCleanupResult struct {
	Profile      string `json:"profile"`
	SessionCount int    `json:"sessionCount"`
	RemovedCount int    `json:"removedCount"`
	SkippedCount int    `json:"skippedCount,omitempty"`
	FailedCount  int    `json:"failedCount"`
	Message      string `json:"message,omitempty"`
}

// GuardTraceKind captures internal action ordering for one guard cycle.
type GuardTraceKind string

const (
	GuardTraceBindingAudit   GuardTraceKind = "binding-audit"
	GuardTraceBindingRepair  GuardTraceKind = "binding-repair"
	GuardTracePortalLogin    GuardTraceKind = "portal-login"
	GuardTraceSessionOffline GuardTraceKind = "session-offline"
)

// GuardTraceEvent records one internal guard action in occurrence order.
type GuardTraceEvent struct {
	Kind          GuardTraceKind
	Message       string
	BindingOK     bool
	InternetOK    bool
	PortalLoginOK bool
	RecoveryStep  string
	Action        string
	HolderProfile string
	TargetProfile string
	Profile       string
	Scope         string
	SessionCount  int
	RemovedCount  int
	SkippedCount  int
	FailedCount   int
}

// EnsureOnlineResult summarizes one aggressive ensure-online chain.
type EnsureOnlineResult struct {
	DesiredProfile            string                    `json:"desiredProfile"`
	InitialProbe              *LocalIPSelection         `json:"initialProbe,omitempty"`
	RetryProbe                *LocalIPSelection         `json:"retryProbe,omitempty"`
	FinalRetryProbe           *LocalIPSelection         `json:"finalRetryProbe,omitempty"`
	FirstPortalLoginOK        bool                      `json:"firstPortalLoginOk"`
	FirstPortalLoginMsg       string                    `json:"firstPortalLoginMessage,omitempty"`
	SecondPortalLoginOK       bool                      `json:"secondPortalLoginOk"`
	SecondPortalLoginMsg      string                    `json:"secondPortalLoginMessage,omitempty"`
	ThirdPortalLoginAttempted bool                      `json:"thirdPortalLoginAttempted,omitempty"`
	ThirdPortalLoginOK        bool                      `json:"thirdPortalLoginOk,omitempty"`
	ThirdPortalLoginMsg       string                    `json:"thirdPortalLoginMessage,omitempty"`
	BindingRepairAttempted    bool                      `json:"bindingRepairAttempted"`
	BindingRepairOK           bool                      `json:"bindingRepairOk"`
	BindingRepairMessage      string                    `json:"bindingRepairMessage,omitempty"`
	BindingRepair             *BindingRepairResult      `json:"bindingRepair,omitempty"`
	OfflineCleanupAttempted   bool                      `json:"offlineCleanupAttempted,omitempty"`
	OfflineCleanupOK          bool                      `json:"offlineCleanupOk,omitempty"`
	OfflineCleanupMessage     string                    `json:"offlineCleanupMessage,omitempty"`
	OfflineCleanup            *OfflineCleanupResult     `json:"offlineCleanup,omitempty"`
	PortalPayload             *kernel.Portal802Response `json:"portalPayload,omitempty"`
	InternetOK                bool                      `json:"internetOk"`
	InternetMessage           string                    `json:"internetMessage,omitempty"`
	RecoveryStep              string                    `json:"recoveryStep"`
	Trace                     []GuardTraceEvent         `json:"-"`
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
	InitialProbe       *LocalIPSelection    `json:"initialProbe,omitempty"`
	RetryProbe         *LocalIPSelection    `json:"retryProbe,omitempty"`
	BindingRepair      *BindingRepairResult `json:"bindingRepair,omitempty"`
	EnsureOnline       *EnsureOnlineResult  `json:"ensureOnline,omitempty"`
	Trace              []GuardTraceEvent    `json:"-"`
}

// GuardEnvironment contains the dependencies needed for guard workflows.
type GuardEnvironment struct {
	Accounts    map[string]Credentials
	Broadband   BroadbandCredentials
	PortalISP   string
	Factory     GuardClientFactory
	Prober      GuardProber
	AfterRepair func(context.Context) error
}

const repairSettleDelay = time.Second

// RepairBinding ensures the desired profile owns the configured mobile broadband credentials.
func RepairBinding(ctx context.Context, env GuardEnvironment, targetProfile string) (*kernel.OperationResult[BindingRepairResult], error) {
	targetAccount, ok := env.Accounts[targetProfile]
	if !ok {
		return nil, &kernel.OpError{Op: "workflow.guard.repairBinding", Message: fmt.Sprintf("target profile %q not found", targetProfile), Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "guard.targetProfile", Value: targetProfile}}
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

	targetOffline, offlineErr := cleanupProfileOnlineSessions(ctx, env, targetProfile, targetClient, "target-before-bind", nil)
	if offlineErr != nil {
		return &kernel.OperationResult[BindingRepairResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: fmt.Sprintf("failed to clear target %s online sessions before binding", targetProfile),
			Data: &BindingRepairResult{
				TargetProfile: targetProfile,
				Action:        "target-offline-failed",
				TargetOffline: targetOffline,
			},
		}, offlineErr
	}

	holderProfile := ""
	var holderClient GuardSelfClient
	for profile, account := range env.Accounts {
		if profile == targetProfile {
			continue
		}
		candidateClient, err := env.Factory.NewSelf()
		if err != nil {
			continue
		}
		if _, err := candidateClient.Login(ctx, account.Username, account.Password); err != nil {
			continue
		}
		holderBinding, err := candidateClient.GetOperatorBinding(ctx)
		if err != nil || holderBinding.Data == nil {
			continue
		}
		if holderBinding.Data.MobileAccount != env.Broadband.Account {
			continue
		}
		holderProfile = profile
		holderClient = candidateClient
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
					TargetOffline: targetOffline,
				},
			}, err
		}
		break
	}

	if _, err := targetClient.BindOperator(ctx, map[string]string{
		"FLDEXTRA3": env.Broadband.Account,
		"FLDEXTRA4": env.Broadband.Password,
	}, true, false); err != nil {
		result := &BindingRepairResult{
			TargetProfile: targetProfile,
			HolderProfile: holderProfile,
			Action:        "target-bind-failed",
			TargetOffline: targetOffline,
		}
		message := fmt.Sprintf("failed to bind target %s", targetProfile)
		if holderProfile != "" && holderClient != nil {
			result.RollbackAttempted = true
			result.RollbackProfile = holderProfile
			if _, rollbackErr := holderClient.BindOperator(ctx, map[string]string{
				"FLDEXTRA3": env.Broadband.Account,
				"FLDEXTRA4": env.Broadband.Password,
			}, true, false); rollbackErr != nil {
				result.Action = "target-bind-failed-rollback-failed"
				result.RollbackOK = false
				result.RollbackMessage = rollbackErr.Error()
				message = fmt.Sprintf("failed to bind target %s; rollback to %s failed", targetProfile, holderProfile)
				err = fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
			} else {
				result.Action = "target-bind-failed-rolled-back"
				result.RollbackOK = true
				result.RollbackMessage = fmt.Sprintf("binding restored to %s", holderProfile)
				message = fmt.Sprintf("failed to bind target %s; rolled back to %s", targetProfile, holderProfile)
			}
		}
		return &kernel.OperationResult[BindingRepairResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: message,
			Data:    result,
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
			TargetOffline: targetOffline,
		},
	}, nil
}

func cleanupConfiguredOnlineSessions(ctx context.Context, env GuardEnvironment, scope string, skipIPs map[string]struct{}) (*OfflineCleanupResult, error) {
	result := &OfflineCleanupResult{Scope: scope}
	profiles := make([]string, 0, len(env.Accounts))
	for profile := range env.Accounts {
		profiles = append(profiles, profile)
	}
	sort.Strings(profiles)

	var firstErr error
	for _, profile := range profiles {
		profileResult, err := cleanupProfileOnlineSessions(ctx, env, profile, nil, scope, skipIPs)
		if profileResult != nil {
			result.Profiles = append(result.Profiles, profileResult.Profiles...)
			result.SessionCount += profileResult.SessionCount
			result.RemovedCount += profileResult.RemovedCount
			result.SkippedCount += profileResult.SkippedCount
			result.FailedCount += profileResult.FailedCount
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	result.Message = offlineCleanupMessage(result)
	return result, firstErr
}

func cleanupProfileOnlineSessions(ctx context.Context, env GuardEnvironment, profile string, client GuardSelfClient, scope string, skipIPs map[string]struct{}) (*OfflineCleanupResult, error) {
	result := &OfflineCleanupResult{Scope: scope}
	profileResult := ProfileOfflineCleanupResult{Profile: profile}
	defer func() {
		result.Profiles = []ProfileOfflineCleanupResult{profileResult}
		result.SessionCount = profileResult.SessionCount
		result.RemovedCount = profileResult.RemovedCount
		result.SkippedCount = profileResult.SkippedCount
		result.FailedCount = profileResult.FailedCount
		result.Message = offlineCleanupMessage(result)
	}()

	account, ok := env.Accounts[profile]
	if !ok {
		profileResult.FailedCount = 1
		profileResult.Message = "profile not configured"
		return result, &kernel.OpError{Op: "workflow.guard.offlineSessions", Message: fmt.Sprintf("profile %q not found", profile), Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "guard.offline.profile", Value: profile}}
	}
	if client == nil {
		created, err := env.Factory.NewSelf()
		if err != nil {
			profileResult.FailedCount = 1
			profileResult.Message = "create self client failed"
			return result, &kernel.OpError{Op: "workflow.guard.offlineSessions", Message: "create self client failed", Err: err}
		}
		client = created
		if _, err := client.Login(ctx, account.Username, account.Password); err != nil {
			profileResult.FailedCount = 1
			profileResult.Message = "self login failed"
			return result, &kernel.OpError{Op: "workflow.guard.offlineSessions", Message: fmt.Sprintf("self login failed for profile %s", profile), Err: err}
		}
	}

	online, err := client.GetOnlineList(ctx)
	if err != nil {
		profileResult.FailedCount = 1
		profileResult.Message = "online list failed"
		return result, &kernel.OpError{Op: "workflow.guard.offlineSessions", Message: fmt.Sprintf("online list failed for profile %s", profile), Err: err}
	}
	sessions := []kernel.OnlineSession{}
	if online != nil && online.Data != nil {
		for _, session := range *online.Data {
			if strings.TrimSpace(session.SessionID) != "" {
				sessions = append(sessions, session)
			}
		}
	}
	profileResult.SessionCount = len(sessions)
	if len(sessions) == 0 {
		profileResult.Message = "no online sessions"
		return result, nil
	}

	var firstErr error
	for _, session := range sessions {
		if shouldSkipOnlineSession(session, skipIPs) {
			profileResult.SkippedCount++
			continue
		}
		offline, err := client.ForceOffline(ctx, session.SessionID)
		if err != nil {
			if errors.Is(err, kernel.ErrGuardedCapability) && offline != nil && strings.Contains(offline.Message, "not present") {
				profileResult.RemovedCount++
				continue
			}
			profileResult.FailedCount++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		profileResult.RemovedCount++
	}
	if firstErr != nil {
		profileResult.Message = "some online sessions failed to go offline"
		return result, firstErr
	}
	if profileResult.RemovedCount == 0 && profileResult.SkippedCount > 0 {
		profileResult.Message = "only current/local online sessions present"
		return result, nil
	}
	profileResult.Message = "online sessions removed"
	return result, nil
}

func offlineCleanupMessage(result *OfflineCleanupResult) string {
	if result == nil {
		return ""
	}
	if result.SessionCount == 0 && result.FailedCount == 0 {
		return "no online sessions to clear"
	}
	if result.SkippedCount == result.SessionCount && result.FailedCount == 0 {
		return fmt.Sprintf("only current/local online sessions present, skipped %d", result.SkippedCount)
	}
	if result.FailedCount > 0 {
		return fmt.Sprintf("cleared %d/%d online sessions, skipped %d, %d failed", result.RemovedCount, result.SessionCount, result.SkippedCount, result.FailedCount)
	}
	return fmt.Sprintf("cleared %d online sessions, skipped %d", result.RemovedCount, result.SkippedCount)
}

func shouldSkipOnlineSession(session kernel.OnlineSession, skipIPs map[string]struct{}) bool {
	if len(skipIPs) == 0 {
		return false
	}
	ip := strings.TrimSpace(session.IP)
	if ip == "" {
		return false
	}
	_, ok := skipIPs[ip]
	return ok
}

func localProbeIPs(probes ...*LocalIPSelection) map[string]struct{} {
	ips := map[string]struct{}{}
	for _, probe := range probes {
		if probe == nil {
			continue
		}
		for _, ip := range []string{probe.SelectedIP, probe.RoutedIP} {
			if ip = strings.TrimSpace(ip); ip != "" {
				ips[ip] = struct{}{}
			}
		}
	}
	if len(ips) == 0 {
		return nil
	}
	return ips
}

// EnsureOnline aggressively restores connectivity for the desired profile.
func EnsureOnline(ctx context.Context, env GuardEnvironment, targetProfile string, forcePortalLogin bool) (*kernel.OperationResult[EnsureOnlineResult], error) {
	account, ok := env.Accounts[targetProfile]
	if !ok {
		return nil, &kernel.OpError{Op: "workflow.guard.ensureOnline", Message: fmt.Sprintf("target profile %q not found", targetProfile), Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "guard.targetProfile", Value: targetProfile}}
	}
	result := &EnsureOnlineResult{
		DesiredProfile: targetProfile,
		RecoveryStep:   "no-ip",
	}
	_ = forcePortalLogin

	initialProbe, err := env.Prober.DetectLocalIPv4(ctx)
	if err != nil || strings.TrimSpace(initialProbe.SelectedIP) == "" {
		return &kernel.OperationResult[EnsureOnlineResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: "unable to detect local IPv4",
			Data:    result,
		}, &kernel.OpError{Op: "workflow.guard.ensureOnline", Message: "unable to detect local IPv4", Err: err}
	}
	result.InitialProbe = &initialProbe

	portalClient, err := env.Factory.NewPortal()
	if err != nil {
		return nil, &kernel.OpError{Op: "workflow.guard.ensureOnline", Message: "create portal client failed", Err: err}
	}
	first, firstErr := portalClient.Login802(ctx, account.Username, account.Password, initialProbe.SelectedIP, env.PortalISP)
	result.FirstPortalLoginOK = firstErr == nil
	if first != nil {
		result.FirstPortalLoginMsg = first.Message
		result.PortalPayload = first.Data
	}
	if strings.TrimSpace(result.FirstPortalLoginMsg) == "" && firstErr != nil {
		result.FirstPortalLoginMsg = portalErrorMessage(firstErr)
	}
	internetOK, internetMessage := env.Prober.CheckConnectivity(ctx)
	result.InternetOK = internetOK
	result.InternetMessage = internetMessage
	result.Trace = append(result.Trace, GuardTraceEvent{
		Kind:          GuardTracePortalLogin,
		Message:       firstPortalTraceMessage(result),
		InternetOK:    internetOK,
		PortalLoginOK: result.FirstPortalLoginOK,
		RecoveryStep:  "portal-login",
	})
	if internetOK {
		result.RecoveryStep = "portal-login"
		message := "portal login restored connectivity"
		if firstErr != nil {
			message = "connectivity available after portal attempt"
		}
		return &kernel.OperationResult[EnsureOnlineResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: true,
			Message: message,
			Data:    result,
		}, nil
	}

	repair, repairErr := RepairBinding(ctx, env, targetProfile)
	result.BindingRepairAttempted = true
	if repair != nil {
		result.BindingRepairOK = repair.Success
		result.BindingRepairMessage = repair.Message
		result.BindingRepair = repair.Data
		result.Trace = append(result.Trace, bindingTraceEvent(repair))
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

	if err := waitAfterRepair(ctx, env.AfterRepair); err != nil {
		result.RecoveryStep = "binding-repair-settle-failed"
		return &kernel.OperationResult[EnsureOnlineResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: "binding repair completed but settle wait failed before retry login",
			Data:    result,
		}, err
	}

	retryProbe, err := env.Prober.DetectLocalIPv4(ctx)
	if err != nil || strings.TrimSpace(retryProbe.SelectedIP) == "" {
		retryProbe = initialProbe
	}
	result.RetryProbe = &retryProbe
	second, secondErr := portalClient.Login802(ctx, account.Username, account.Password, retryProbe.SelectedIP, env.PortalISP)
	result.SecondPortalLoginOK = secondErr == nil
	if second != nil {
		result.SecondPortalLoginMsg = second.Message
		result.PortalPayload = second.Data
	}
	if strings.TrimSpace(result.SecondPortalLoginMsg) == "" && secondErr != nil {
		result.SecondPortalLoginMsg = portalErrorMessage(secondErr)
	}
	internetOK, internetMessage = env.Prober.CheckConnectivity(ctx)
	result.InternetOK = internetOK
	result.InternetMessage = internetMessage
	result.RecoveryStep = "binding-repair-then-portal-login"
	result.Trace = append(result.Trace, GuardTraceEvent{
		Kind:          GuardTracePortalLogin,
		Message:       secondPortalTraceMessage(result),
		InternetOK:    internetOK,
		PortalLoginOK: result.SecondPortalLoginOK,
		RecoveryStep:  result.RecoveryStep,
	})
	if internetOK {
		return &kernel.OperationResult[EnsureOnlineResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: true,
			Message: "binding repair and portal login restored connectivity",
			Data:    result,
		}, nil
	}

	cleanup, cleanupErr := cleanupConfiguredOnlineSessions(ctx, env, "post-portal-offline", localProbeIPs(result.InitialProbe, result.RetryProbe))
	result.OfflineCleanupAttempted = true
	result.OfflineCleanupOK = cleanupErr == nil
	if cleanup != nil {
		result.OfflineCleanup = cleanup
		result.OfflineCleanupMessage = cleanup.Message
		if trace := offlineCleanupTraceEvent(cleanup, "offline-sessions-then-portal-login"); trace != nil {
			result.Trace = append(result.Trace, *trace)
		}
	}
	if cleanup != nil && cleanup.RemovedCount > 0 {
		if err := waitAfterRepair(ctx, env.AfterRepair); err != nil {
			result.RecoveryStep = "offline-sessions-settle-failed"
			return &kernel.OperationResult[EnsureOnlineResult]{
				Level:   kernel.EvidenceConfirmed,
				Success: false,
				Message: "online sessions cleared but settle wait failed before final login",
				Data:    result,
			}, err
		}
		finalProbe, err := env.Prober.DetectLocalIPv4(ctx)
		if err != nil || strings.TrimSpace(finalProbe.SelectedIP) == "" {
			finalProbe = retryProbe
		}
		result.FinalRetryProbe = &finalProbe
		third, thirdErr := portalClient.Login802(ctx, account.Username, account.Password, finalProbe.SelectedIP, env.PortalISP)
		result.ThirdPortalLoginAttempted = true
		result.ThirdPortalLoginOK = thirdErr == nil
		if third != nil {
			result.ThirdPortalLoginMsg = third.Message
			result.PortalPayload = third.Data
		}
		if strings.TrimSpace(result.ThirdPortalLoginMsg) == "" && thirdErr != nil {
			result.ThirdPortalLoginMsg = portalErrorMessage(thirdErr)
		}
		internetOK, internetMessage = env.Prober.CheckConnectivity(ctx)
		result.InternetOK = internetOK
		result.InternetMessage = internetMessage
		result.RecoveryStep = "offline-sessions-then-portal-login"
		result.Trace = append(result.Trace, GuardTraceEvent{
			Kind:          GuardTracePortalLogin,
			Message:       thirdPortalTraceMessage(result),
			InternetOK:    internetOK,
			PortalLoginOK: result.ThirdPortalLoginOK,
			RecoveryStep:  result.RecoveryStep,
		})
		if internetOK {
			return &kernel.OperationResult[EnsureOnlineResult]{
				Level:   kernel.EvidenceConfirmed,
				Success: true,
				Message: "online sessions cleared and portal login restored connectivity",
				Data:    result,
			}, nil
		}
		return &kernel.OperationResult[EnsureOnlineResult]{
			Level:   kernel.EvidenceConfirmed,
			Success: false,
			Message: "connectivity still unavailable after online-session cleanup",
			Data:    result,
		}, firstNonNil(thirdErr, cleanupErr, secondErr)
	}
	return &kernel.OperationResult[EnsureOnlineResult]{
		Level:   kernel.EvidenceConfirmed,
		Success: false,
		Message: "connectivity still unavailable after portal retry",
		Data:    result,
	}, firstNonNil(secondErr, cleanupErr)
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
			result.Trace = append(result.Trace, forceSwitchTraceEvent(repair, "portal-login"))
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
			result.Trace = append(result.Trace, GuardTraceEvent{
				Kind:         GuardTraceBindingAudit,
				Message:      repair.Message,
				BindingOK:    repair.Success,
				RecoveryStep: "healthy",
			})
			if trace := bindingRepairTraceEvent(repair, "healthy"); trace != nil {
				result.Trace = append(result.Trace, *trace)
			}
		}
		if err != nil {
			result.BindingOK = false
			result.RecoveryStep = "degraded-binding-repair"
			updateLastTraceRecoveryStep(result.Trace, result.RecoveryStep)
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
	dst.InitialProbe = src.InitialProbe
	dst.RetryProbe = src.RetryProbe
	dst.PortalLoginOK = finalPortalLoginOK(src)
	dst.InternetOK = src.InternetOK
	dst.InternetMessage = src.InternetMessage
	dst.PortalLoginMessage = portalMessageFromEnsure(src)
	dst.RecoveryStep = src.RecoveryStep
	dst.Trace = append(dst.Trace, src.Trace...)
	if src.BindingRepairAttempted {
		dst.BindingOK = src.BindingRepairOK
		dst.BindingMessage = src.BindingRepairMessage
		if src.BindingRepair != nil {
			dst.BindingRepair = src.BindingRepair
		}
	}
}

func finalPortalLoginOK(src *EnsureOnlineResult) bool {
	if src == nil {
		return false
	}
	if src.ThirdPortalLoginAttempted {
		return src.ThirdPortalLoginOK
	}
	if src.BindingRepairAttempted || src.RetryProbe != nil || strings.TrimSpace(src.SecondPortalLoginMsg) != "" {
		return src.SecondPortalLoginOK
	}
	return src.FirstPortalLoginOK
}

func portalMessageFromEnsure(src *EnsureOnlineResult) string {
	switch {
	case src.ThirdPortalLoginMsg != "":
		return src.ThirdPortalLoginMsg
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

func portalErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var opErr *kernel.OpError
	if errors.As(err, &opErr) && strings.TrimSpace(opErr.Message) != "" {
		return opErr.Message
	}
	return err.Error()
}

func waitAfterRepair(ctx context.Context, waitFn func(context.Context) error) error {
	if waitFn != nil {
		return waitFn(ctx)
	}
	timer := time.NewTimer(repairSettleDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func firstPortalTraceMessage(result *EnsureOnlineResult) string {
	if result == nil {
		return ""
	}
	if strings.TrimSpace(result.FirstPortalLoginMsg) != "" {
		return result.FirstPortalLoginMsg
	}
	return result.InternetMessage
}

func secondPortalTraceMessage(result *EnsureOnlineResult) string {
	if result == nil {
		return ""
	}
	if strings.TrimSpace(result.SecondPortalLoginMsg) != "" {
		return result.SecondPortalLoginMsg
	}
	return result.InternetMessage
}

func thirdPortalTraceMessage(result *EnsureOnlineResult) string {
	if result == nil {
		return ""
	}
	if strings.TrimSpace(result.ThirdPortalLoginMsg) != "" {
		return result.ThirdPortalLoginMsg
	}
	return result.InternetMessage
}

func bindingTraceEvent(repair *kernel.OperationResult[BindingRepairResult]) GuardTraceEvent {
	if repair == nil {
		return GuardTraceEvent{}
	}
	if trace := bindingRepairTraceEvent(repair, "binding-repair-then-portal-login"); trace != nil {
		return *trace
	}
	return GuardTraceEvent{
		Kind:         GuardTraceBindingAudit,
		Message:      repair.Message,
		BindingOK:    repair.Success,
		RecoveryStep: "binding-repair-then-portal-login",
	}
}

func forceSwitchTraceEvent(repair *kernel.OperationResult[BindingRepairResult], recoveryStep string) GuardTraceEvent {
	if repair == nil {
		return GuardTraceEvent{}
	}
	if trace := bindingRepairTraceEvent(repair, recoveryStep); trace != nil {
		return *trace
	}
	return GuardTraceEvent{
		Kind:         GuardTraceBindingAudit,
		Message:      repair.Message,
		BindingOK:    repair.Success,
		RecoveryStep: recoveryStep,
	}
}

func offlineCleanupTraceEvent(cleanup *OfflineCleanupResult, recoveryStep string) *GuardTraceEvent {
	if cleanup == nil || cleanup.SessionCount == 0 {
		return nil
	}
	return &GuardTraceEvent{
		Kind:         GuardTraceSessionOffline,
		Message:      cleanup.Message,
		RecoveryStep: recoveryStep,
		Scope:        cleanup.Scope,
		SessionCount: cleanup.SessionCount,
		RemovedCount: cleanup.RemovedCount,
		SkippedCount: cleanup.SkippedCount,
		FailedCount:  cleanup.FailedCount,
	}
}

func bindingRepairTraceEvent(repair *kernel.OperationResult[BindingRepairResult], recoveryStep string) *GuardTraceEvent {
	if repair == nil || repair.Data == nil {
		return nil
	}
	if strings.TrimSpace(repair.Data.Action) == "" || repair.Data.Action == "already-correct" {
		return nil
	}
	return &GuardTraceEvent{
		Kind:          GuardTraceBindingRepair,
		Message:       repair.Message,
		BindingOK:     repair.Success,
		RecoveryStep:  recoveryStep,
		Action:        repair.Data.Action,
		HolderProfile: repair.Data.HolderProfile,
		TargetProfile: repair.Data.TargetProfile,
	}
}

func updateLastTraceRecoveryStep(trace []GuardTraceEvent, recoveryStep string) {
	if len(trace) == 0 {
		return
	}
	trace[len(trace)-1].RecoveryStep = recoveryStep
}

func firstNonNil(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
