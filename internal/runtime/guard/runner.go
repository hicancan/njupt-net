package guard

import (
	"context"
	"io"
	"os"
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
	settings  Settings
	store     *StateStore
	logger    *Logger
	scheduler *Scheduler
	prober    *Probe
	state     runnerState
	now       func() time.Time
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
	return &Runner{
		settings:  settings,
		store:     store,
		logger:    NewLogger(writer),
		scheduler: scheduler,
		prober:    NewProbe(minDuration(settings.ProbeInterval, 1500*time.Millisecond)),
		now:       time.Now,
	}, nil
}

// Run starts the foreground guard loop.
func (r *Runner) Run(ctx context.Context, replaceLegacy bool) error {
	if replaceLegacy {
		supervisor := NewSupervisor(r.store, "", "")
		killed, err := supervisor.StopLegacy()
		if err != nil {
			return err
		}
		if killed {
			r.logger.Printf("legacy python guard stopped before Go runtime takeover")
		}
	}

	if err := r.store.WritePID(r.store.WorkerPIDFile(), os.Getpid()); err != nil {
		return err
	}
	defer r.store.RemovePID(r.store.WorkerPIDFile())

	r.logger.Printf("starting Go guard in %s", r.store.StateDir())
	for {
		select {
		case <-ctx.Done():
			r.logger.Printf("guard stopped")
			return nil
		default:
		}

		cycleStarted := time.Now()
		status, err := r.executeCycle(ctx)
		status.CycleElapsedSeconds = time.Since(cycleStarted).Seconds()
		status.Running = true
		status.LogPath = r.store.CurrentLogPath()
		status.Timestamp = r.now().In(r.settings.Location).Format("2006-01-02 15:04:05")
		if err := r.store.WriteStatus(*status); err != nil {
			return err
		}
		r.logger.Printf(
			"cycle=%d desired=%s window=%s binding_ok=%t internet_ok=%t portal_ok=%t step=%s elapsed=%.2fs",
			status.CycleIndex,
			status.DesiredProfile,
			status.ScheduleWindow,
			status.BindingOK,
			status.InternetOK,
			status.PortalLoginOK,
			status.RecoveryStep,
			status.CycleElapsedSeconds,
		)

		wait := r.settings.ProbeInterval - time.Since(cycleStarted)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			r.logger.Printf("guard stopped")
			return nil
		case <-timer.C:
		}
		if err != nil {
			continue
		}
	}
}

// Once runs a single cycle and returns the typed status.
func (r *Runner) Once(ctx context.Context, replaceLegacy bool) (*Status, error) {
	if replaceLegacy {
		supervisor := NewSupervisor(r.store, "", "")
		_, _ = supervisor.StopLegacy()
	}
	status, err := r.executeCycle(ctx)
	status.Running = false
	status.LogPath = r.store.CurrentLogPath()
	status.Timestamp = r.now().In(r.settings.Location).Format("2006-01-02 15:04:05")
	return status, err
}

func (r *Runner) executeCycle(ctx context.Context) (*Status, error) {
	r.state.cycleIndex++
	now := r.now().In(r.settings.Location)
	decision := r.scheduler.Decide(now)
	forceSwitch := r.state.currentProfile == "" || decision.Profile != r.state.currentProfile
	forceBinding := forceSwitch || r.state.lastBindingAudit.IsZero() || now.Sub(r.state.lastBindingAudit) >= r.settings.BindingCheckInterval

	env := workflow.GuardEnvironment{
		Accounts:  r.settings.Accounts,
		Broadband: r.settings.Broadband,
		PortalISP: r.settings.PortalISP,
		Factory: workflow.RealGuardClientFactory{
			SelfBaseURL:           r.settings.SelfBaseURL,
			PortalBaseURL:         r.settings.PortalBaseURL,
			PortalFallbackBaseURL: r.settings.PortalFallbackBaseURL,
			SelfTimeout:           r.settings.SelfTimeout,
			PortalTimeout:         r.settings.PortalTimeout,
			InsecureTLS:           r.settings.InsecureTLS,
		},
		Prober: r.prober,
	}
	cycle, err := workflow.GuardCycle(ctx, env, workflow.GuardCycleInput{
		DesiredProfile:    decision.Profile,
		ScheduleWindow:    decision.Window,
		ForceSwitch:       forceSwitch,
		ForceBindingCheck: forceBinding,
	})
	status := &Status{
		Running:            true,
		DesiredProfile:     cycle.DesiredProfile,
		ScheduleWindow:     cycle.ScheduleWindow,
		BindingOK:          cycle.BindingOK,
		BindingMessage:     cycle.BindingMessage,
		InitialInternetOK:  cycle.InitialInternetOK,
		InitialInternetMsg: cycle.InitialInternetMsg,
		InternetOK:         cycle.InternetOK,
		InternetMessage:    cycle.InternetMessage,
		PortalLoginOK:      cycle.PortalLoginOK,
		PortalLoginMessage: cycle.PortalLoginMessage,
		RecoveryStep:       cycle.RecoveryStep,
		CycleIndex:         r.state.cycleIndex,
		LocalIP:            cycle.LocalIP,
		RetryLocalIP:       cycle.RetryLocalIP,
	}

	if forceBinding && cycle.BindingOK {
		r.state.lastBindingAudit = now
	}
	if forceSwitch && cycle.BindingOK && cycle.InternetOK && cycle.PortalLoginOK {
		r.state.currentProfile = decision.Profile
		r.state.lastSwitchAt = now
	}
	if !r.state.lastSwitchAt.IsZero() {
		status.LastSwitchAt = r.state.lastSwitchAt.Format(time.RFC3339)
	}
	return status, err
}
