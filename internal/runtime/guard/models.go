package guard

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hicancan/njupt-net-cli/internal/config"
	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

// Overrides are CLI-level guard setting overrides.
type Overrides struct {
	StateDir             string
	ProbeInterval        int
	BindingCheckInterval int
	Timezone             string
	DayProfile           string
	NightProfile         string
	NightStart           string
	NightEnd             string
}

// Settings are the fully resolved runtime settings for the Go guard.
type Settings struct {
	StateDir              string
	ProbeInterval         time.Duration
	BindingCheckInterval  time.Duration
	Location              *time.Location
	Accounts              map[string]config.AccountConfig
	Broadband             config.BroadbandConfig
	SelfBaseURL           string
	SelfTimeout           time.Duration
	PortalBaseURL         string
	PortalFallbackBaseURL string
	PortalTimeout         time.Duration
	PortalISP             string
	InsecureTLS           bool
	Schedule              ScheduleConfig
}

// Health summarizes the final health state of the guard runtime.
type Health string

const (
	HealthHealthy  Health = "healthy"
	HealthDegraded Health = "degraded"
	HealthStopped  Health = "stopped"
)

// ProbeStatus captures a typed local IPv4 selection used for Portal calls.
type ProbeStatus struct {
	SelectedIP      string `json:"selectedIp,omitempty"`
	RoutedIP        string `json:"routedIp,omitempty"`
	SelectionReason string `json:"selectionReason,omitempty"`
}

// BindingRepairStatus captures one binding repair outcome.
type BindingRepairStatus struct {
	Attempted     bool   `json:"attempted"`
	OK            bool   `json:"ok"`
	Action        string `json:"action,omitempty"`
	HolderProfile string `json:"holderProfile,omitempty"`
	TargetProfile string `json:"targetProfile,omitempty"`
}

// BindingStatus captures final binding state for one cycle.
type BindingStatus struct {
	Audited bool                 `json:"audited"`
	OK      bool                 `json:"ok"`
	Message string               `json:"message,omitempty"`
	Repair  *BindingRepairStatus `json:"repair,omitempty"`
}

// ConnectivityStatus captures the initial and final internet observations.
type ConnectivityStatus struct {
	InitialOK      bool         `json:"initialOk"`
	InitialMessage string       `json:"initialMessage,omitempty"`
	FinalOK        bool         `json:"finalOk"`
	FinalMessage   string       `json:"finalMessage,omitempty"`
	Probe          *ProbeStatus `json:"probe,omitempty"`
}

// PortalStatus captures whether Portal was attempted and with which local IP selection.
type PortalStatus struct {
	Attempted    bool         `json:"attempted"`
	OK           bool         `json:"ok"`
	Message      string       `json:"message,omitempty"`
	InitialProbe *ProbeStatus `json:"initialProbe,omitempty"`
	RetryProbe   *ProbeStatus `json:"retryProbe,omitempty"`
}

// CycleStatus captures cycle-local state-machine information.
type CycleStatus struct {
	Index           int    `json:"index,omitempty"`
	RecoveryStep    string `json:"recoveryStep,omitempty"`
	LastSwitchAt    string `json:"lastSwitchAt,omitempty"`
	SwitchTriggered bool   `json:"switchTriggered"`
	SwitchCompleted bool   `json:"switchCompleted"`
}

// TimingStatus captures persisted timestamp and elapsed runtime per cycle.
type TimingStatus struct {
	Timestamp      string  `json:"timestamp,omitempty"`
	ElapsedSeconds float64 `json:"elapsedSeconds,omitempty"`
}

// LogStatus captures the current log pointer.
type LogStatus struct {
	Path string `json:"path,omitempty"`
}

// Status is the persisted guard state exposed by `njupt-net guard status`.
type Status struct {
	Running        bool               `json:"running"`
	Health         Health             `json:"health,omitempty"`
	DesiredProfile string             `json:"desiredProfile,omitempty"`
	ScheduleWindow string             `json:"scheduleWindow,omitempty"`
	Binding        BindingStatus      `json:"binding"`
	Connectivity   ConnectivityStatus `json:"connectivity"`
	Portal         PortalStatus       `json:"portal"`
	Cycle          CycleStatus        `json:"cycle"`
	Timing         TimingStatus       `json:"timing"`
	Log            LogStatus          `json:"log"`
}

// ControlResult summarizes start/stop/status command results.
type ControlResult struct {
	Running      bool   `json:"running"`
	PID          int    `json:"pid,omitempty"`
	LogPath      string `json:"logPath,omitempty"`
	StateDir     string `json:"stateDir,omitempty"`
	LegacyKilled bool   `json:"legacyKilled,omitempty"`
}

// BuildSettings resolves runtime settings from config and CLI overrides.
func BuildSettings(cfg *config.Config, overrides Overrides, insecureTLS bool) (Settings, error) {
	locationName := chooseString(overrides.Timezone, cfg.Guard.Timezone)
	location, err := time.LoadLocation(locationName)
	if err != nil {
		return Settings{}, &kernel.OpError{Op: "guard.settings", Message: fmt.Sprintf("invalid timezone %q", locationName), Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "guard.timezone", Value: locationName}}
	}

	broadband, err := cfg.ResolveBroadband()
	if err != nil {
		return Settings{}, err
	}

	stateDir := chooseString(overrides.StateDir, cfg.Guard.StateDir)
	if strings.TrimSpace(stateDir) == "" {
		stateDir = filepath.Join("dist", "guard")
	}
	probeInterval := choosePositive(overrides.ProbeInterval, cfg.Guard.ProbeIntervalSeconds)
	bindingInterval := choosePositive(overrides.BindingCheckInterval, cfg.Guard.BindingCheckIntervalSeconds)
	if probeInterval <= 0 || bindingInterval <= 0 {
		return Settings{}, &kernel.OpError{Op: "guard.settings", Message: "probe and binding intervals must be positive", Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "guard.intervals", Hint: "probeIntervalSeconds and bindingCheckIntervalSeconds must be positive"}}
	}

	schedule := ScheduleConfig{
		DayProfile:   chooseString(overrides.DayProfile, cfg.Guard.Schedule.DayProfile),
		NightProfile: chooseString(overrides.NightProfile, cfg.Guard.Schedule.NightProfile),
		NightStart:   chooseString(overrides.NightStart, cfg.Guard.Schedule.NightStart),
		NightEnd:     chooseString(overrides.NightEnd, cfg.Guard.Schedule.NightEnd),
	}
	if err := schedule.Validate(); err != nil {
		return Settings{}, err
	}
	for _, profile := range []string{schedule.DayProfile, schedule.NightProfile} {
		if _, ok := cfg.Accounts[profile]; !ok {
			return Settings{}, &kernel.OpError{
				Op:      "guard.settings",
				Message: fmt.Sprintf("guard profile %q is not configured in accounts", profile),
				Err:     kernel.ErrInvalidConfig,
				ProblemDetails: kernel.ConfigProblemDetails{
					Field: "guard.schedule.profile",
					Value: profile,
				},
			}
		}
	}

	settings := Settings{
		StateDir:              stateDir,
		ProbeInterval:         time.Duration(probeInterval) * time.Second,
		BindingCheckInterval:  time.Duration(bindingInterval) * time.Second,
		Location:              location,
		Accounts:              cfg.Accounts,
		Broadband:             broadband,
		SelfBaseURL:           cfg.Self.BaseURL,
		SelfTimeout:           time.Duration(cfg.Self.TimeoutSeconds) * time.Second,
		PortalBaseURL:         cfg.Portal.BaseURL,
		PortalFallbackBaseURL: firstFallback(cfg.Portal.FallbackBaseURLs),
		PortalTimeout:         time.Duration(cfg.Portal.TimeoutSeconds) * time.Second,
		PortalISP:             cfg.Portal.ISP,
		InsecureTLS:           insecureTLS || cfg.Portal.InsecureTLS,
		Schedule:              schedule,
	}
	return settings, nil
}

func chooseString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func choosePositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func firstFallback(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
