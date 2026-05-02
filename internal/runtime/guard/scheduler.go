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
	DayProfile              string
	NightProfile            string
	NightStart              string
	NightEnd                string
	SkipNightSwitchWeekdays []string
}

// Decision is one fully resolved profile decision.
type Decision struct {
	Profile string
	Window  string
}

// Scheduler resolves the target profile for a specific local time.
type Scheduler struct {
	config                  ScheduleConfig
	nightStartMinutes       int
	nightEndMinutes         int
	skipNightSwitchWeekdays map[time.Weekday]struct{}
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
	skipNightSwitchWeekdays, err := parseWeekdays(cfg.SkipNightSwitchWeekdays)
	if err != nil {
		return nil, err
	}
	return &Scheduler{
		config:                  cfg,
		nightStartMinutes:       start,
		nightEndMinutes:         end,
		skipNightSwitchWeekdays: skipNightSwitchWeekdays,
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
			return &kernel.OpError{Op: "guard.schedule", Message: fmt.Sprintf("%s is required", label), Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "guard.schedule." + label}}
		}
	}
	if _, err := parseClockMinutes(c.NightStart); err != nil {
		return err
	}
	if _, err := parseClockMinutes(c.NightEnd); err != nil {
		return err
	}
	if _, err := parseWeekdays(c.SkipNightSwitchWeekdays); err != nil {
		return err
	}
	return nil
}

// Decide returns the current target profile and logical schedule window.
func (s *Scheduler) Decide(now time.Time) Decision {
	local := now
	minutes := local.Hour()*60 + local.Minute()
	if minutes >= s.nightStartMinutes {
		if s.shouldSkipNightSwitch(local.Weekday()) {
			return Decision{Profile: s.config.DayProfile, Window: windowNight}
		}
		return Decision{Profile: s.config.NightProfile, Window: windowNight}
	}
	if minutes < s.nightEndMinutes {
		nightStartDay := local.AddDate(0, 0, -1).Weekday()
		if s.shouldSkipNightSwitch(nightStartDay) {
			return Decision{Profile: s.config.DayProfile, Window: windowNight}
		}
		return Decision{Profile: s.config.NightProfile, Window: windowNight}
	}
	return Decision{Profile: s.config.DayProfile, Window: windowDay}
}

func (s *Scheduler) shouldSkipNightSwitch(weekday time.Weekday) bool {
	if s == nil || len(s.skipNightSwitchWeekdays) == 0 {
		return false
	}
	_, ok := s.skipNightSwitchWeekdays[weekday]
	return ok
}

func parseClockMinutes(raw string) (int, error) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 2 {
		return 0, &kernel.OpError{Op: "guard.schedule", Message: fmt.Sprintf("invalid clock value %q", raw), Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "guard.schedule.clock", Value: raw}}
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, &kernel.OpError{Op: "guard.schedule", Message: fmt.Sprintf("invalid clock value %q", raw), Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "guard.schedule.clock", Value: raw}}
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, &kernel.OpError{Op: "guard.schedule", Message: fmt.Sprintf("invalid clock value %q", raw), Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "guard.schedule.clock", Value: raw}}
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, &kernel.OpError{Op: "guard.schedule", Message: fmt.Sprintf("invalid clock value %q", raw), Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "guard.schedule.clock", Value: raw}}
	}
	return hour*60 + minute, nil
}

func parseWeekdays(values []string) (map[time.Weekday]struct{}, error) {
	weekdays := make(map[time.Weekday]struct{}, len(values))
	for _, value := range values {
		weekday, err := parseWeekday(value)
		if err != nil {
			return nil, err
		}
		weekdays[weekday] = struct{}{}
	}
	return weekdays, nil
}

func parseWeekday(raw string) (time.Weekday, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "sunday", "sun", "0", "7", "周日", "星期日", "星期天", "日", "天":
		return time.Sunday, nil
	case "monday", "mon", "1", "周一", "星期一", "一":
		return time.Monday, nil
	case "tuesday", "tue", "2", "周二", "星期二", "二":
		return time.Tuesday, nil
	case "wednesday", "wed", "3", "周三", "星期三", "三":
		return time.Wednesday, nil
	case "thursday", "thu", "4", "周四", "星期四", "四":
		return time.Thursday, nil
	case "friday", "fri", "5", "周五", "星期五", "五":
		return time.Friday, nil
	case "saturday", "sat", "6", "周六", "星期六", "六":
		return time.Saturday, nil
	default:
		return time.Sunday, &kernel.OpError{Op: "guard.schedule", Message: fmt.Sprintf("invalid weekday value %q", raw), Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "guard.schedule.skipNightSwitchWeekdays", Value: raw}}
	}
}
