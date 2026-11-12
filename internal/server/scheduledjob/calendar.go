package scheduledjob

import (
	"time"
	_ "time/tzdata" // Calendar zones are available on minimal hosts.

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	jobmodel "github.com/josemiguelmelo/orkest/internal/server/scheduledjob/model"
)

// Next selects a calendar minute strictly after the cutoff. UTC scanning skips
// nonexistent wall times. Rejecting repeated local minutes fires a fold once.
func Next(s jobmodel.Schedule, after time.Time) (time.Time, error) {
	if (s.Frequency != "daily" && s.Frequency != "weekly") || s.TimeZone == "" || s.TimeZone == "Local" || s.Hour < 0 || s.Hour > 23 || s.Minute < 0 || s.Minute > 59 || s.Weekday < 0 || s.Weekday > 6 {
		return time.Time{}, modelerrors.ErrInvalid
	}
	zone, err := time.LoadLocation(s.TimeZone)
	if err != nil {
		return time.Time{}, modelerrors.ErrInvalid
	}
	start := after.UTC().Truncate(time.Minute).Add(time.Minute)
	for t := start; t.Before(start.AddDate(0, 0, 9)); t = t.Add(time.Minute) {
		local := t.In(zone)
		if local.Hour() != s.Hour || local.Minute() != s.Minute || (s.Frequency == "weekly" && int(local.Weekday()) != s.Weekday) {
			continue
		}
		repeated := false
		for earlier := t.Add(-3 * time.Hour); earlier.Before(t); earlier = earlier.Add(time.Minute) {
			l := earlier.In(zone)
			if l.Year() == local.Year() && l.YearDay() == local.YearDay() && l.Hour() == local.Hour() && l.Minute() == local.Minute() {
				repeated = true
				break
			}
		}
		if !repeated {
			return t, nil
		}
	}
	return time.Time{}, modelerrors.ErrInvalid
}

// Due coalesces arbitrary downtime using a bounded calendar search.
func Due(s jobmodel.Schedule, next, now time.Time) (time.Time, time.Time, error) {
	cutoff := now.Add(-8 * 24 * time.Hour)
	if next.After(cutoff) {
		cutoff = next.Add(-time.Second)
	}
	latest := next
	for {
		candidate, err := Next(s, cutoff)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		if candidate.After(now) {
			return latest, candidate, nil
		}
		latest, cutoff = candidate, candidate
	}
}

// OccurrenceState keeps timing and overlap policy outside persistence.
func OccurrenceState(occupied, allowed, validResources, missed bool) (string, string) {
	if !allowed {
		return "needs_attention", "authorization_revoked"
	}
	if !validResources {
		return "needs_attention", "repository_unavailable"
	}
	if missed {
		return "missed", "outside_lateness_window"
	}
	if occupied {
		return "skipped", "previous_invocation_unresolved"
	}
	return "pending", ""
}
