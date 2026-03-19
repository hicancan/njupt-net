package guard

import (
	"testing"
	"time"
)

func TestSchedulerDecide(t *testing.T) {
	scheduler, err := NewScheduler(ScheduleConfig{
		WeekdayDayProfile:   "B",
		WeekdayNightProfile: "W",
		WeekdayNightStart:   "23:30",
		WeekdayNightEnd:     "07:00",
		WeekendProfile:      "B",
		OvernightMode:       "calendar-day",
	})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	location := time.FixedZone("CST", 8*60*60)
	tests := []struct {
		name    string
		when    time.Time
		profile string
		window  string
	}{
		{"monday 00:30", time.Date(2026, 3, 16, 0, 30, 0, 0, location), "W", windowWeekdayNight},
		{"monday 07:00", time.Date(2026, 3, 16, 7, 0, 0, 0, location), "B", windowWeekdayDay},
		{"monday 23:30", time.Date(2026, 3, 16, 23, 30, 0, 0, location), "W", windowWeekdayNight},
		{"friday 23:45", time.Date(2026, 3, 20, 23, 45, 0, 0, location), "W", windowWeekdayNight},
		{"saturday 00:00", time.Date(2026, 3, 21, 0, 0, 0, 0, location), "B", windowWeekend},
		{"sunday 23:45", time.Date(2026, 3, 22, 23, 45, 0, 0, location), "B", windowWeekend},
		{"monday next 00:00", time.Date(2026, 3, 23, 0, 0, 0, 0, location), "W", windowWeekdayNight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := scheduler.Decide(tt.when)
			if decision.Profile != tt.profile || decision.Window != tt.window {
				t.Fatalf("unexpected decision: %#v", decision)
			}
		})
	}
}
