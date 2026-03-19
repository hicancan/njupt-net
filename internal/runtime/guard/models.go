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

// Status is the persisted guard state exposed by `njupt-net guard status`.
type Status struct {
	Running             bool    `json:"running"`
	DesiredProfile      string  `json:"desiredProfile,omitempty"`
	ScheduleWindow      string  `json:"scheduleWindow,omitempty"`
	BindingOK           bool    `json:"bindingOk"`
	BindingMessage      string  `json:"bindingMessage,omitempty"`
	InitialInternetOK   bool    `json:"initialInternetOk"`
	InitialInternetMsg  string  `json:"initialInternetMessage,omitempty"`
	InternetOK          bool    `json:"internetOk"`
	InternetMessage     string  `json:"internetMessage,omitempty"`
	PortalLoginOK       bool    `json:"portalLoginOk"`
	PortalLoginMessage  string  `json:"portalLoginMessage,omitempty"`
	RecoveryStep        string  `json:"recoveryStep,omitempty"`
	CycleIndex          int     `json:"cycleIndex,omitempty"`
	LastSwitchAt        string  `json:"lastSwitchAt,omitempty"`
	CycleElapsedSeconds float64 `json:"cycleElapsedSeconds,omitempty"`
	LogPath             string  `json:"logPath,omitempty"`
	Timestamp           string  `json:"timestamp,omitempty"`
	LocalIP             string  `json:"localIp,omitempty"`
	RetryLocalIP        string  `json:"retryLocalIp,omitempty"`
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
		return Settings{}, &kernel.OpError{Op: "guard.settings", Message: fmt.Sprintf("invalid timezone %q", locationName), Err: kernel.ErrInvalidConfig}
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
		return Settings{}, &kernel.OpError{Op: "guard.settings", Message: "probe and binding intervals must be positive", Err: kernel.ErrInvalidConfig}
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
