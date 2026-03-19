package guard

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/hicancan/njupt-net-cli/internal/workflow"
)

type runnerState struct {
	currentProfile   string
	lastSwitchAt     time.Time
	lastBindingAudit time.Time
	cycleIndex       int
}

// Runner executes the long-lived Go guard loop.
type Runner struct {
	settings   Settings
	store      *StateStore
	recorder   *Recorder
	scheduler  *Scheduler
	prober     workflow.GuardProber
	state      runnerState
	now        func() time.Time
	closeEvent func()
	guardCycle func(ctx context.Context, env workflow.GuardEnvironment, input workflow.GuardCycleInput) (*workflow.GuardCycleResult, error)
}

// NewRunner creates a guard runner with the supported runtime dependencies.
func NewRunner(settings Settings, store *StateStore, writer io.Writer) (*Runner, error) {
	scheduler, err := NewScheduler(settings.Schedule)
	if err != nil {
		return nil, err
	}
	if writer == nil {
		writer = os.Stdout
	}
	eventWriter, closeEvent, err := openEventWriter(store.CurrentEventPath())
	if err != nil {
		return nil, err
	}
	return &Runner{
		settings:   settings,
		store:      store,
		recorder:   NewRecorder(writer, eventWriter),
		scheduler:  scheduler,
		prober:     NewProbe(minDuration(settings.ProbeInterval, 1500*time.Millisecond)),
		now:        time.Now,
		closeEvent: closeEvent,
		guardCycle: workflow.GuardCycle,
	}, nil
}

// Run starts the foreground guard loop.
func (r *Runner) Run(ctx context.Context, replaceLegacy bool) error {
	defer r.close()
	defer func() {
		if recovered := recover(); recovered != nil {
			r.emit(Event{
				Kind:    EventFatal,
				Message: fmt.Sprintf("guard panic: %v", recovered),
			})
		}
	}()
	r.store.ClearStopRequest()
	_ = r.store.PruneLogs(10)

	if replaceLegacy {
		supervisor := NewSupervisor(r.store, "", "")
		killed, err := supervisor.StopLegacy()
		if err != nil {
			return err
		}
		if killed {
			r.emit(Event{
				Kind:    EventStartup,
				Message: "legacy python guard stopped before Go runtime takeover",
			})
		}
	}

	if err := r.store.WritePID(r.store.WorkerPIDFile(), os.Getpid()); err != nil {
		return err
	}
	defer func() {
		r.store.RemovePID(r.store.WorkerPIDFile())
		r.store.ClearStopRequest()
	}()

	r.emit(Event{
		Kind:    EventStartup,
		Message: "starting Go guard",
		Details: StartupEventDetails{
			StateDir: r.store.StateDir(),
		},
	})
	for {
		if reason, ok := r.store.ReadStopRequest(); ok {
			r.emit(Event{
				Kind:    EventShutdown,
				Message: "graceful stop request observed",
				Details: ShutdownEventDetails{
					Reason: firstNonEmpty(reason, "guard stop requested"),
				},
			})
			return nil
		}
		select {
		case <-ctx.Done():
			r.emit(Event{Kind: EventShutdown, Message: "guard stopped by context cancellation"})
			return nil
		default:
		}

		cycleStarted := time.Now()
		status, err := r.executeCycle(ctx)
		status.Timing.ElapsedSeconds = time.Since(cycleStarted).Seconds()
		status.Running = true
		status.Log.Path = r.store.CurrentLogPath()
		status.Timing.Timestamp = r.now().In(r.settings.Location).Format("2006-01-02 15:04:05")
		if err := r.store.WriteStatus(*status); err != nil {
			return err
		}

		wait := r.settings.ProbeInterval - time.Since(cycleStarted)
		if !r.waitForNextCycle(ctx, wait) {
			r.emit(Event{Kind: EventShutdown, Message: "guard stopped while waiting for next cycle"})
			return nil
		}
		if err != nil {
			continue
		}
	}
}

// Once runs a single cycle and returns the typed status.
func (r *Runner) Once(ctx context.Context, replaceLegacy bool) (*Status, error) {
	defer r.close()
	if replaceLegacy {
		supervisor := NewSupervisor(r.store, "", "")
		_, _ = supervisor.StopLegacy()
	}
	status, err := r.executeCycle(ctx)
	status.Running = false
	status.Log.Path = r.store.CurrentLogPath()
	status.Timing.Timestamp = r.now().In(r.settings.Location).Format("2006-01-02 15:04:05")
	return status, err
}

func (r *Runner) executeCycle(ctx context.Context) (*Status, error) {
	r.state.cycleIndex++
	now := r.now().In(r.settings.Location)
	decision := r.scheduler.Decide(now)
	forceSwitch := r.state.currentProfile == "" || decision.Profile != r.state.currentProfile
	forceBinding := forceSwitch || r.state.lastBindingAudit.IsZero() || now.Sub(r.state.lastBindingAudit) >= r.settings.BindingCheckInterval
	localProbe := detectCycleProbe(ctx, r.prober)

	env := workflow.GuardEnvironment{
		Accounts:  toWorkflowAccounts(r.settings.Accounts),
		Broadband: toWorkflowBroadband(r.settings.Broadband),
		PortalISP: r.settings.PortalISP,
		Factory:   newClientFactory(r.settings),
		Prober:    r.prober,
	}
	cycleFn := r.guardCycle
	if cycleFn == nil {
		cycleFn = workflow.GuardCycle
	}
	cycle, err := cycleFn(ctx, env, workflow.GuardCycleInput{
		DesiredProfile:    decision.Profile,
		ScheduleWindow:    decision.Window,
		ForceSwitch:       forceSwitch,
		ForceBindingCheck: forceBinding,
	})
	status := &Status{
		Running:        true,
		DesiredProfile: cycle.DesiredProfile,
		ScheduleWindow: cycle.ScheduleWindow,
		Binding: BindingStatus{
			Audited: forceBinding,
			OK:      cycle.BindingOK,
			Message: cycle.BindingMessage,
		},
		Connectivity: ConnectivityStatus{
			InitialOK:      cycle.InitialInternetOK,
			InitialMessage: cycle.InitialInternetMsg,
			FinalOK:        cycle.InternetOK,
			FinalMessage:   cycle.InternetMessage,
			Probe:          probeStatusFromSelection(localProbe),
		},
		Portal: PortalStatus{
			Attempted:    portalLoginAttemptedFromCycle(cycle),
			OK:           cycle.PortalLoginOK,
			Message:      cycle.PortalLoginMessage,
			InitialProbe: probeStatusFromSelection(cycle.InitialProbe),
			RetryProbe:   probeStatusFromSelection(cycle.RetryProbe),
		},
		Cycle: CycleStatus{
			Index:           r.state.cycleIndex,
			RecoveryStep:    cycle.RecoveryStep,
			SwitchTriggered: forceSwitch,
		},
	}
	if cycle.BindingRepair != nil {
		status.Binding.Repair = &BindingRepairStatus{
			Attempted:     true,
			OK:            cycle.BindingOK,
			Action:        cycle.BindingRepair.Action,
			HolderProfile: cycle.BindingRepair.HolderProfile,
			TargetProfile: cycle.BindingRepair.TargetProfile,
		}
	}
	status.Health = deriveHealth(status)

	if forceBinding && cycle.BindingOK {
		r.state.lastBindingAudit = now
	}
	if forceSwitch && cycle.BindingOK && cycle.InternetOK {
		r.state.currentProfile = decision.Profile
		r.state.lastSwitchAt = now
		status.Cycle.SwitchCompleted = true
	}
	if !r.state.lastSwitchAt.IsZero() {
		status.Cycle.LastSwitchAt = r.state.lastSwitchAt.Format(time.RFC3339)
	}
	r.recordCycle(decision, forceSwitch, forceBinding, cycle, status, err)
	return status, err
}

func (r *Runner) waitForNextCycle(ctx context.Context, wait time.Duration) bool {
	if wait < 0 {
		wait = 0
	}
	deadline := time.NewTimer(wait)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return true
		case <-ticker.C:
			if r.store.StopRequested() {
				return false
			}
		}
	}
}

func (r *Runner) emit(event Event) {
	if r == nil || r.recorder == nil {
		return
	}
	r.recorder.Emit(event)
}

func (r *Runner) recordCycle(decision Decision, forceSwitch, forceBinding bool, cycle *workflow.GuardCycleResult, status *Status, cycleErr error) {
	if status == nil {
		return
	}
	base := Event{
		CycleIndex:     status.Cycle.Index,
		DesiredProfile: status.DesiredProfile,
		ScheduleWindow: status.ScheduleWindow,
	}
	if forceSwitch && status.Health == HealthHealthy {
		event := base
		event.Kind = EventScheduleSwitch
		event.Message = fmt.Sprintf("scheduled target switched to %s", decision.Profile)
		event.Details = ScheduleSwitchEventDetails{
			BindingOK:     status.Binding.OK,
			InternetOK:    status.Connectivity.FinalOK,
			PortalLoginOK: status.Portal.OK,
			RecoveryStep:  status.Cycle.RecoveryStep,
		}
		r.emit(event)
	}
	if forceBinding {
		event := base
		event.Kind = EventBindingAudit
		event.Message = status.Binding.Message
		event.Details = BindingAuditEventDetails{
			BindingOK:    status.Binding.OK,
			RecoveryStep: status.Cycle.RecoveryStep,
		}
		r.emit(event)
	}
	if status.Portal.Attempted {
		event := base
		event.Kind = EventPortalLogin
		event.Message = status.Portal.Message
		event.Details = PortalLoginEventDetails{
			InternetOK:    status.Connectivity.FinalOK,
			PortalLoginOK: status.Portal.OK,
			RecoveryStep:  status.Cycle.RecoveryStep,
		}
		r.emit(event)
	}
	if bindingRepairPerformed(cycle) {
		event := base
		event.Kind = EventBindingRepair
		event.Message = status.Binding.Message
		if cycle != nil && cycle.BindingRepair != nil {
			event.Details = BindingRepairEventDetails{
				Action:        cycle.BindingRepair.Action,
				BindingOK:     status.Binding.OK,
				HolderProfile: cycle.BindingRepair.HolderProfile,
				RecoveryStep:  status.Cycle.RecoveryStep,
				TargetProfile: cycle.BindingRepair.TargetProfile,
			}
		}
		r.emit(event)
	}
	if status.Health == HealthDegraded {
		event := base
		event.Kind = EventDegraded
		event.Message = firstNonEmpty(status.Connectivity.FinalMessage, status.Portal.Message, status.Binding.Message, "guard cycle degraded")
		event.Details = DegradedEventDetails{
			BindingOK:     status.Binding.OK,
			InternetOK:    status.Connectivity.FinalOK,
			PortalLoginOK: status.Portal.OK,
			RecoveryStep:  status.Cycle.RecoveryStep,
		}
		if cycleErr != nil {
			event.Details = DegradedEventDetails{
				BindingOK:     status.Binding.OK,
				Error:         cycleErr.Error(),
				InternetOK:    status.Connectivity.FinalOK,
				PortalLoginOK: status.Portal.OK,
				RecoveryStep:  status.Cycle.RecoveryStep,
			}
		}
		r.emit(event)
	}
}

func portalLoginAttemptedFromCycle(cycle *workflow.GuardCycleResult) bool {
	if cycle == nil {
		return false
	}
	if cycle.EnsureOnline != nil {
		ensure := cycle.EnsureOnline
		if ensure.FirstPortalLoginOK || ensure.SecondPortalLoginOK || ensure.PortalPayload != nil {
			return true
		}
		if strings.TrimSpace(ensure.FirstPortalLoginMsg) != "" || strings.TrimSpace(ensure.SecondPortalLoginMsg) != "" {
			return true
		}
	}
	message := strings.TrimSpace(cycle.PortalLoginMessage)
	switch message {
	case "", "portal login not needed", "portal login skipped because switch binding repair failed":
		return false
	default:
		return true
	}
}

func bindingRepairPerformed(cycle *workflow.GuardCycleResult) bool {
	if cycle == nil || cycle.BindingRepair == nil {
		return false
	}
	action := strings.TrimSpace(cycle.BindingRepair.Action)
	return action != "" && action != "already-correct"
}

func deriveHealth(status *Status) Health {
	if status == nil {
		return HealthDegraded
	}
	if status.Binding.OK && status.Connectivity.FinalOK {
		return HealthHealthy
	}
	return HealthDegraded
}

func probeStatusFromSelection(selection *workflow.LocalIPSelection) *ProbeStatus {
	if selection == nil {
		return nil
	}
	return &ProbeStatus{
		SelectedIP:      selection.SelectedIP,
		RoutedIP:        selection.RoutedIP,
		SelectionReason: selection.SelectionReason,
	}
}

func detectCycleProbe(ctx context.Context, prober workflow.GuardProber) *workflow.LocalIPSelection {
	if prober == nil {
		return nil
	}
	selection, err := prober.DetectLocalIPv4(ctx)
	if err != nil {
		return nil
	}
	if strings.TrimSpace(selection.SelectedIP) == "" && strings.TrimSpace(selection.RoutedIP) == "" {
		return nil
	}
	return &selection
}

func (r *Runner) close() {
	if r != nil && r.closeEvent != nil {
		r.closeEvent()
		r.closeEvent = nil
	}
}

func openEventWriter(path string) (io.Writer, func(), error) {
	if strings.TrimSpace(path) == "" {
		return nil, func() {}, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return file, func() { _ = file.Close() }, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
