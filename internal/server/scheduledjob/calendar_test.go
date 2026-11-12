package scheduledjob

import (
	"testing"
	"time"

	jobmodel "github.com/josemiguelmelo/orkest/internal/server/scheduledjob/model"
)

func TestCalendarDST(t *testing.T) {
	tests := []struct {
		name, after, want string
		s                 jobmodel.Schedule
	}{
		{"daily", "2026-11-12T09:00:00Z", "2026-11-13T09:00:00Z", jobmodel.Schedule{Frequency: "daily", TimeZone: "Europe/Lisbon", Hour: 9}},
		{"spring skips nonexistent minute", "2026-03-28T12:00:00Z", "2026-03-30T00:30:00Z", jobmodel.Schedule{Frequency: "daily", TimeZone: "Europe/Lisbon", Hour: 1, Minute: 30}},
		{"fold first instant", "2026-10-24T12:00:00Z", "2026-10-25T00:30:00Z", jobmodel.Schedule{Frequency: "daily", TimeZone: "Europe/Lisbon", Hour: 1, Minute: 30}},
		{"fold never repeats", "2026-10-25T00:30:00Z", "2026-10-26T01:30:00Z", jobmodel.Schedule{Frequency: "daily", TimeZone: "Europe/Lisbon", Hour: 1, Minute: 30}},
		{"weekly", "2026-11-12T00:00:00Z", "2026-11-16T09:00:00Z", jobmodel.Schedule{Frequency: "weekly", TimeZone: "UTC", Hour: 9, Weekday: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			after, _ := time.Parse(time.RFC3339, tt.after)
			got, err := Next(tt.s, after)
			if err != nil || got.Format(time.RFC3339) != tt.want {
				t.Fatalf("got %s, %v; want %s", got, err, tt.want)
			}
		})
	}
}

func TestCalendarInvalid(t *testing.T) {
	for _, s := range []jobmodel.Schedule{{Frequency: "cron", TimeZone: "UTC"}, {Frequency: "daily", TimeZone: "unknown/zone"}, {Frequency: "daily", TimeZone: "UTC", Hour: 24}, {Frequency: "daily", TimeZone: "Local"}} {
		if _, err := Next(s, time.Now()); err == nil {
			t.Fatalf("accepted invalid calendar: %#v", s)
		}
	}
}

func TestDowntimeCoalescesLatest(t *testing.T) {
	s := jobmodel.Schedule{Frequency: "daily", TimeZone: "UTC", Hour: 9}
	next := time.Date(2020, 1, 1, 9, 0, 0, 0, time.UTC)
	now := time.Date(2026, 11, 12, 9, 15, 0, 0, time.UTC)
	latest, future, err := Due(s, next, now)
	if err != nil || !latest.Equal(time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)) || !future.Equal(time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("coalescing: %v %v %v", latest, future, err)
	}
}
