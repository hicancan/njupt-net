package guard

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

const (
	windowDay   = "day"
	windowNight = "night"
)

// ScheduleConfig is the validated day/night switching model for the guard runtime.
type ScheduleConfig struct {
	DayProfile   string
	NightProfile string
	NightStart   string
	NightEnd     string
}

// Decision is one fully resolved profile decision.
type Decision struct {
	Profile string
	Window  string
}

// Scheduler resolves the target profile for a specific local time.
type Scheduler struct {
	config            ScheduleConfig
	nightStartMinutes int
	nightEndMinutes   int
}

// NewScheduler validates and compiles the schedule configuration.
func NewScheduler(cfg ScheduleConfig) (*Scheduler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	start, err := parseClockMinutes(cfg.NightStart)
	if err != nil {
		return nil, err
	}
	end, err := parseClockMinutes(cfg.NightEnd)
	if err != nil {
		return nil, err
	}
	return &Scheduler{
		config:            cfg,
		nightStartMinutes: start,
		nightEndMinutes:   end,
	}, nil
}

// Validate ensures the schedule is internally coherent.
func (c ScheduleConfig) Validate() error {
	for label, value := range map[string]string{
		"dayProfile":   c.DayProfile,
		"nightProfile": c.NightProfile,
		"nightStart":   c.NightStart,
		"nightEnd":     c.NightEnd,
	} {
		if strings.TrimSpace(value) == "" {
			return &kernel.OpError{Op: "guard.schedule", Message: fmt.Sprintf("%s is required", label), Err: kernel.ErrInvalidConfig}
		}
	}
	if _, err := parseClockMinutes(c.NightStart); err != nil {
		return err
	}
	if _, err := parseClockMinutes(c.NightEnd); err != nil {
		return err
	}
	return nil
}

// Decide returns the current target profile and logical schedule window.
func (s *Scheduler) Decide(now time.Time) Decision {
	local := now
	minutes := local.Hour()*60 + local.Minute()
	if minutes >= s.nightStartMinutes || minutes < s.nightEndMinutes {
		return Decision{Profile: s.config.NightProfile, Window: windowNight}
	}
	return Decision{Profile: s.config.DayProfile, Window: windowDay}
}

func parseClockMinutes(raw string) (int, error) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 2 {
		return 0, &kernel.OpError{Op: "guard.schedule", Message: fmt.Sprintf("invalid clock value %q", raw), Err: kernel.ErrInvalidConfig}
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, &kernel.OpError{Op: "guard.schedule", Message: fmt.Sprintf("invalid clock value %q", raw), Err: kernel.ErrInvalidConfig}
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, &kernel.OpError{Op: "guard.schedule", Message: fmt.Sprintf("invalid clock value %q", raw), Err: kernel.ErrInvalidConfig}
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, &kernel.OpError{Op: "guard.schedule", Message: fmt.Sprintf("invalid clock value %q", raw), Err: kernel.ErrInvalidConfig}
	}
	return hour*60 + minute, nil
}
