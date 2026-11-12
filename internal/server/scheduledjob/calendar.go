package scheduledjob

import (
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Calendar zones are available on minimal hosts.

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	jobmodel "github.com/josemiguelmelo/orkest/internal/server/scheduledjob/model"
	"github.com/robfig/cron/v3"
)

var standardCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// Next selects a calendar minute strictly after the cutoff. UTC scanning skips
// nonexistent wall times. Rejecting repeated local minutes fires a fold once.
func Next(s jobmodel.Schedule, after time.Time) (time.Time, error) {
	if s.TimeZone == "" || s.TimeZone == "Local" {
		return time.Time{}, modelerrors.ErrInvalid
	}
	zone, err := time.LoadLocation(s.TimeZone)
	if err != nil {
		return time.Time{}, modelerrors.ErrInvalid
	}
	if s.Frequency == "cron" {
		return nextCron(s, after, zone)
	}
	if (s.Frequency != "daily" && s.Frequency != "weekly") || s.CronExpression != "" || s.Hour < 0 || s.Hour > 23 || s.Minute < 0 || s.Minute > 59 || s.Weekday < 0 || s.Weekday > 6 {
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

func nextCron(s jobmodel.Schedule, after time.Time, zone *time.Location) (time.Time, error) {
	expression := strings.TrimSpace(s.CronExpression)
	if expression == "" || len(expression) > 128 {
		return time.Time{}, modelerrors.ErrInvalid
	}
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return time.Time{}, modelerrors.ErrInvalid
	}
	weekday, err := normalizeCronWeekday(fields[4])
	if err != nil {
		return time.Time{}, modelerrors.ErrInvalid
	}
	fields[4] = weekday
	schedule, err := standardCronParser.Parse("CRON_TZ=" + s.TimeZone + " " + strings.Join(fields, " "))
	if err != nil {
		return time.Time{}, modelerrors.ErrInvalid
	}
	for cursor := after.UTC(); ; {
		candidate := schedule.Next(cursor)
		if candidate.IsZero() {
			return time.Time{}, modelerrors.ErrInvalid
		}
		candidate = candidate.UTC()
		local := candidate.In(zone)
		if !repeatedWallMinute(candidate, local, zone) {
			return candidate, nil
		}
		cursor = candidate
	}
}

// Robfig uses the common 0-6 weekday range. Crontab also accepts 7 for Sunday;
// normalize that alias, including ranges ending at 7, before parsing.
func normalizeCronWeekday(field string) (string, error) {
	parts := strings.Split(field, ",")
	for index, part := range parts {
		base, stepText, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			parsed, err := strconv.Atoi(stepText)
			if err != nil || parsed < 1 || parsed > 7 {
				return "", modelerrors.ErrInvalid
			}
			step = parsed
		}
		if base == "*" {
			continue
		}
		fromText, toText, isRange := strings.Cut(base, "-")
		if !isRange {
			if strings.EqualFold(base, "7") {
				parts[index] = "0"
			}
			continue
		}
		from, fromOK := parseCronWeekday(fromText)
		to, toOK := parseCronWeekday(toText)
		if !fromOK || !toOK || to < from || to > 7 || from > 7 {
			continue // The cron parser will report invalid names and ranges.
		}
		if to != 7 && from != 7 {
			continue
		}
		values := make([]string, 0, (to-from)/step+1)
		for value := from; value <= to; value += step {
			weekday := value
			if weekday == 7 {
				weekday = 0
			}
			values = append(values, strconv.Itoa(weekday))
		}
		parts[index] = strings.Join(values, ",")
	}
	return strings.Join(parts, ","), nil
}

func parseCronWeekday(value string) (int, bool) {
	if number, err := strconv.Atoi(value); err == nil {
		return number, number >= 0 && number <= 7
	}
	weekdays := map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	day, ok := weekdays[strings.ToLower(value)]
	return day, ok
}

func repeatedWallMinute(candidate, localTime time.Time, zone *time.Location) bool {
	for earlier := candidate.Add(-3 * time.Hour); earlier.Before(candidate); earlier = earlier.Add(time.Minute) {
		previous := earlier.In(zone)
		if previous.Year() == localTime.Year() && previous.YearDay() == localTime.YearDay() && previous.Hour() == localTime.Hour() && previous.Minute() == localTime.Minute() {
			return true
		}
	}
	return false
}

// Due coalesces arbitrary downtime using a bounded calendar search.
func Due(s jobmodel.Schedule, next, now time.Time) (time.Time, time.Time, error) {
	if s.Frequency == "cron" {
		return dueCron(s, next, now)
	}
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

// Cron expressions can have gaps longer than the lateness window (for example,
// a run on the first day of each month). Find the most recent due minute with a
// bounded binary search rather than assuming there is an occurrence every week.
func dueCron(s jobmodel.Schedule, next, now time.Time) (time.Time, time.Time, error) {
	future, err := Next(s, now)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if next.After(now) {
		return next, future, nil
	}
	low, high := next.Add(-time.Minute), now
	for high.Sub(low) > time.Second {
		mid := low.Add(high.Sub(low) / 2)
		candidate, e := Next(s, mid)
		if e != nil {
			return time.Time{}, time.Time{}, e
		}
		if candidate.After(now) {
			high = mid
		} else {
			low = mid
		}
	}
	latest, err := Next(s, low)
	if err != nil || latest.After(now) {
		return time.Time{}, time.Time{}, modelerrors.ErrInvalid
	}
	return latest, future, nil
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
