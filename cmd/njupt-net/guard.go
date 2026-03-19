package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
	runtimeguard "github.com/hicancan/njupt-net-cli/internal/runtime/guard"
	"github.com/spf13/cobra"
)

type guardFlags struct {
	StateDir             string
	ProbeInterval        int
	BindingCheckInterval int
	Timezone             string
	DayProfile           string
	NightProfile         string
	NightStart           string
	NightEnd             string
	WeekendProfile       string
	Replace              bool
	LogFile              string
}

func newGuardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "guard",
		Short: "Go daemon for scheduled account guarding and aggressive recovery",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newGuardRunCmd())
	cmd.AddCommand(newGuardStartCmd())
	cmd.AddCommand(newGuardStopCmd())
	cmd.AddCommand(newGuardStatusCmd())
	cmd.AddCommand(newGuardOnceCmd())
	return cmd
}

func newGuardRunCmd() *cobra.Command {
	var flags guardFlags
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the Go guard in the foreground",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireYes(cmd, "guard run"); err != nil {
				return err
			}
			settings, store, err := loadGuardSettings(cmd, flags)
			if err != nil {
				return err
			}
			createdLogFile := false
			if strings.TrimSpace(flags.LogFile) != "" {
				if err := store.UseLogPath(flags.LogFile); err != nil {
					return err
				}
			} else {
				logPath, err := store.NextLogPath()
				if err != nil {
					return err
				}
				flags.LogFile = logPath
				createdLogFile = true
			}

			writer := io.Writer(os.Stdout)
			closeLog := func() {}
			if createdLogFile {
				logWriter, closer, err := runtimeguard.OpenForegroundWriter(flags.LogFile, os.Stdout)
				if err != nil {
					return err
				}
				writer = logWriter
				closeLog = closer
			} else if strings.TrimSpace(flags.LogFile) != "" {
				logWriter, closer, err := runtimeguard.OpenForegroundWriter(flags.LogFile, nil)
				if err != nil {
					return err
				}
				writer = logWriter
				closeLog = closer
			}
			defer closeLog()

			runner, err := runtimeguard.NewRunner(settings, store, writer)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runner.Run(ctx, flags.Replace)
		},
	}
	bindGuardFlags(cmd, &flags)
	cmd.Flags().StringVar(&flags.LogFile, "log-file", "", "Internal log file path override")
	_ = cmd.Flags().MarkHidden("log-file")
	return cmd
}

func newGuardStartCmd() *cobra.Command {
	var flags guardFlags
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Go guard in the background",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireYes(cmd, "guard start"); err != nil {
				return err
			}
			_, store, err := loadGuardSettings(cmd, flags)
			if err != nil {
				return err
			}
			executable, err := os.Executable()
			if err != nil {
				return err
			}
			workDir, err := os.Getwd()
			if err != nil {
				return err
			}
			supervisor := runtimeguard.NewSupervisor(store, executable, workDir)
			legacyKilled := false
			if flags.Replace {
				legacyKilled, err = supervisor.StopLegacy()
				if err != nil {
					return err
				}
			}
			rootArgs := buildGuardRootArgs()
			runArgs := buildGuardRunArgs(flags)
			result, err := supervisor.Start(cmd.Context(), runtimeguard.BuildRunArgs(rootArgs, runArgs), flags.Replace)
			if result == nil {
				result = &runtimeguard.ControlResult{}
			}
			result.LegacyKilled = legacyKilled
			payload := &kernel.OperationResult[runtimeguard.ControlResult]{
				Level:   kernel.EvidenceConfirmed,
				Success: err == nil && result.Running,
				Message: guardStartMessage(result, legacyKilled),
				Data:    result,
			}
			if renderErr := render(cmd, payload, func(w io.Writer) error {
				return printKV(w,
					payload.Message,
					fmt.Sprintf("running=%t", result.Running),
					fmt.Sprintf("pid=%d", result.PID),
					"logPath="+result.LogPath,
				)
			}); renderErr != nil {
				return renderErr
			}
			return err
		},
	}
	bindGuardFlags(cmd, &flags)
	return cmd
}

func newGuardStopCmd() *cobra.Command {
	var flags guardFlags
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Go guard background process",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireYes(cmd, "guard stop"); err != nil {
				return err
			}
			store, err := loadGuardStore(cmd, flags.StateDir)
			if err != nil {
				return err
			}
			supervisor := runtimeguard.NewSupervisor(store, "", "")
			result, err := supervisor.Stop(cmd.Context())
			payload := &kernel.OperationResult[runtimeguard.ControlResult]{
				Level:   kernel.EvidenceConfirmed,
				Success: err == nil,
				Message: "guard stopped",
				Data:    result,
			}
			if renderErr := render(cmd, payload, func(w io.Writer) error {
				return printKV(w, payload.Message, "running=false")
			}); renderErr != nil {
				return renderErr
			}
			return err
		},
	}
	bindGuardFlags(cmd, &flags)
	return cmd
}

func newGuardStatusCmd() *cobra.Command {
	var flags guardFlags
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Read the current Go guard status file",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := loadGuardStore(cmd, flags.StateDir)
			if err != nil {
				return err
			}
			status, err := store.ReadStatus()
			if err != nil {
				payload := &kernel.OperationResult[runtimeguard.Status]{
					Level:   kernel.EvidenceConfirmed,
					Success: false,
					Message: "guard status file not found",
					Data: &runtimeguard.Status{
						Running: false,
						LogPath: store.CurrentLogPath(),
					},
				}
				if renderErr := render(cmd, payload, func(w io.Writer) error {
					return printKV(w, payload.Message, "running=false")
				}); renderErr != nil {
					return renderErr
				}
				return &kernel.OpError{Op: "guard.status", Message: "guard status file not found", Err: kernel.ErrBusinessFailed}
			}
			supervisor := runtimeguard.NewSupervisor(store, "", "")
			control, _ := supervisor.Status(cmd.Context())
			if control != nil {
				status.Running = control.Running
				if status.LogPath == "" {
					status.LogPath = control.LogPath
				}
			}
			payload := &kernel.OperationResult[runtimeguard.Status]{
				Level:   kernel.EvidenceConfirmed,
				Success: true,
				Message: "guard status loaded",
				Data:    status,
			}
			return render(cmd, payload, func(w io.Writer) error {
				return printKV(w,
					payload.Message,
					fmt.Sprintf("running=%t", status.Running),
					"desiredProfile="+status.DesiredProfile,
					"scheduleWindow="+status.ScheduleWindow,
					fmt.Sprintf("bindingOk=%t", status.BindingOK),
					fmt.Sprintf("internetOk=%t", status.InternetOK),
					fmt.Sprintf("portalLoginOk=%t", status.PortalLoginOK),
					"recoveryStep="+status.RecoveryStep,
					"logPath="+status.LogPath,
				)
			})
		},
	}
	bindGuardFlags(cmd, &flags)
	return cmd
}

func newGuardOnceCmd() *cobra.Command {
	var flags guardFlags
	cmd := &cobra.Command{
		Use:   "once",
		Short: "Run one guard cycle for debugging and inspection",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireYes(cmd, "guard once"); err != nil {
				return err
			}
			settings, store, err := loadGuardSettings(cmd, flags)
			if err != nil {
				return err
			}
			runner, err := runtimeguard.NewRunner(settings, store, os.Stdout)
			if err != nil {
				return err
			}
			status, err := runner.Once(context.Background(), flags.Replace)
			payload := &kernel.OperationResult[runtimeguard.Status]{
				Level:   kernel.EvidenceConfirmed,
				Success: err == nil && status != nil && status.InternetOK,
				Message: "guard cycle completed",
				Data:    status,
			}
			if renderErr := render(cmd, payload, func(w io.Writer) error {
				if status == nil {
					return printKV(w, payload.Message)
				}
				return printKV(w,
					payload.Message,
					"desiredProfile="+status.DesiredProfile,
					"scheduleWindow="+status.ScheduleWindow,
					fmt.Sprintf("bindingOk=%t", status.BindingOK),
					fmt.Sprintf("internetOk=%t", status.InternetOK),
					fmt.Sprintf("portalLoginOk=%t", status.PortalLoginOK),
					"recoveryStep="+status.RecoveryStep,
				)
			}); renderErr != nil {
				return renderErr
			}
			return err
		},
	}
	bindGuardFlags(cmd, &flags)
	return cmd
}

func bindGuardFlags(cmd *cobra.Command, flags *guardFlags) {
	cmd.Flags().StringVar(&flags.StateDir, "state-dir", "", "Guard state directory")
	cmd.Flags().IntVar(&flags.ProbeInterval, "probe-interval", 0, "Connectivity probe interval in seconds")
	cmd.Flags().IntVar(&flags.BindingCheckInterval, "binding-check-interval", 0, "Binding audit interval in seconds")
	cmd.Flags().StringVar(&flags.Timezone, "timezone", "", "IANA timezone for schedule evaluation")
	cmd.Flags().StringVar(&flags.DayProfile, "day-profile", "", "Weekday daytime profile")
	cmd.Flags().StringVar(&flags.NightProfile, "night-profile", "", "Weekday night profile")
	cmd.Flags().StringVar(&flags.NightStart, "night-start", "", "Night window start time (HH:MM)")
	cmd.Flags().StringVar(&flags.NightEnd, "night-end", "", "Night window end time (HH:MM)")
	cmd.Flags().StringVar(&flags.WeekendProfile, "weekend-profile", "", "Weekend profile")
	cmd.Flags().BoolVar(&flags.Replace, "replace", false, "Replace any existing Go guard and stop the legacy Python guard")
}

func loadGuardSettings(cmd *cobra.Command, flags guardFlags) (runtimeguard.Settings, *runtimeguard.StateStore, error) {
	appCtx, err := rootOpts.load(cmd)
	if err != nil {
		return runtimeguard.Settings{}, nil, err
	}
	settings, err := runtimeguard.BuildSettings(appCtx.Config, runtimeguard.Overrides{
		StateDir:             flags.StateDir,
		ProbeInterval:        flags.ProbeInterval,
		BindingCheckInterval: flags.BindingCheckInterval,
		Timezone:             flags.Timezone,
		DayProfile:           flags.DayProfile,
		NightProfile:         flags.NightProfile,
		NightStart:           flags.NightStart,
		NightEnd:             flags.NightEnd,
		WeekendProfile:       flags.WeekendProfile,
	}, appCtx.InsecureTLS)
	if err != nil {
		return runtimeguard.Settings{}, nil, err
	}
	store, err := runtimeguard.NewStateStore(settings.StateDir)
	if err != nil {
		return runtimeguard.Settings{}, nil, err
	}
	return settings, store, nil
}

func loadGuardStore(cmd *cobra.Command, stateDir string) (*runtimeguard.StateStore, error) {
	appCtx, err := rootOpts.load(cmd)
	if err != nil {
		return nil, err
	}
	resolvedStateDir := stateDir
	if strings.TrimSpace(resolvedStateDir) == "" {
		resolvedStateDir = appCtx.Config.Guard.StateDir
		if strings.TrimSpace(resolvedStateDir) == "" {
			resolvedStateDir = "dist/guard"
		}
	}
	return runtimeguard.NewStateStore(resolvedStateDir)
}

func buildGuardRootArgs() []string {
	args := []string{}
	if strings.TrimSpace(rootOpts.ConfigPath) != "" {
		args = append(args, "--config", rootOpts.ConfigPath)
	}
	if rootOpts.InsecureTLS {
		args = append(args, "--insecure-tls")
	}
	return args
}

func buildGuardRunArgs(flags guardFlags) []string {
	args := []string{}
	if strings.TrimSpace(flags.StateDir) != "" {
		args = append(args, "--state-dir", flags.StateDir)
	}
	if flags.ProbeInterval > 0 {
		args = append(args, "--probe-interval", fmt.Sprintf("%d", flags.ProbeInterval))
	}
	if flags.BindingCheckInterval > 0 {
		args = append(args, "--binding-check-interval", fmt.Sprintf("%d", flags.BindingCheckInterval))
	}
	if strings.TrimSpace(flags.Timezone) != "" {
		args = append(args, "--timezone", flags.Timezone)
	}
	if strings.TrimSpace(flags.DayProfile) != "" {
		args = append(args, "--day-profile", flags.DayProfile)
	}
	if strings.TrimSpace(flags.NightProfile) != "" {
		args = append(args, "--night-profile", flags.NightProfile)
	}
	if strings.TrimSpace(flags.NightStart) != "" {
		args = append(args, "--night-start", flags.NightStart)
	}
	if strings.TrimSpace(flags.NightEnd) != "" {
		args = append(args, "--night-end", flags.NightEnd)
	}
	if strings.TrimSpace(flags.WeekendProfile) != "" {
		args = append(args, "--weekend-profile", flags.WeekendProfile)
	}
	if strings.TrimSpace(flags.LogFile) != "" {
		args = append(args, "--log-file", flags.LogFile)
	}
	args = append(args, "--yes")
	return args
}

func guardStartMessage(result *runtimeguard.ControlResult, legacyKilled bool) string {
	switch {
	case result == nil:
		return "guard start failed"
	case legacyKilled:
		return "guard started after replacing the legacy python guard"
	case result.Running:
		return "guard started"
	default:
		return "guard not running"
	}
}
