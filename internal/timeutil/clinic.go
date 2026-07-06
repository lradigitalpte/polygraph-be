package timeutil

import (
	"fmt"
	"sync"
	"time"
)

// DubaiUTCOffsetHours is the fixed offset from UTC (no daylight saving).
const DubaiUTCOffsetHours = 4

// ClinicTimezone is the wall-clock timezone for scheduling (Polygraph UAE, Dubai).
const ClinicTimezone = "Asia/Dubai"

// DefaultBusyDurationMinutes is used when an appointment row has no duration set.
const DefaultBusyDurationMinutes = 30

var (
	clinicLocation     *time.Location
	clinicLocationOnce sync.Once
)

func ClinicLocation() *time.Location {
	clinicLocationOnce.Do(func() {
		loc, err := time.LoadLocation(ClinicTimezone)
		if err != nil {
			// Dubai has no DST; fixed UTC+4 is the clinic offset.
			clinicLocation = time.FixedZone("GST", DubaiUTCOffsetHours*60*60)
			return
		}
		clinicLocation = loc
	})
	return clinicLocation
}

func ParseClinicDate(date string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", date, ClinicLocation())
}

func ClinicDayBounds(date string) (time.Time, time.Time, error) {
	parsed, err := ParseClinicDate(date)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	dayStart := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, ClinicLocation())
	return dayStart, dayStart.Add(24 * time.Hour), nil
}

func FormatClock(t time.Time) string {
	return fmt.Sprintf("%02d:%02d", UtcToDubaiHour(t), UtcToDubaiMinute(t))
}

func ClockMinutes(t time.Time) int {
	return UtcToDubaiHour(t)*60 + UtcToDubaiMinute(t)
}

// UtcToDubaiHour returns the Dubai wall-clock hour for a UTC instant (UTC hour + 4).
func UtcToDubaiHour(t time.Time) int {
	return t.In(ClinicLocation()).Hour()
}

// UtcToDubaiMinute returns the Dubai wall-clock minute for a UTC instant.
func UtcToDubaiMinute(t time.Time) int {
	return t.In(ClinicLocation()).Minute()
}

func BusyDurationMinutes(duration int) int {
	if duration > 0 {
		return duration
	}
	return DefaultBusyDurationMinutes
}
