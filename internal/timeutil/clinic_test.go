package timeutil

import (
	"testing"
	"time"
)

func TestFormatClockUsesClinicTimezone(t *testing.T) {
	// 08:30 Dubai = 04:30 UTC
	utc := time.Date(2026, 7, 7, 4, 30, 0, 0, time.UTC)
	if got := FormatClock(utc); got != "08:30" {
		t.Fatalf("FormatClock() = %q, want 08:30", got)
	}
}

func TestClinicDayBounds(t *testing.T) {
	start, end, err := ClinicDayBounds("2026-07-07")
	if err != nil {
		t.Fatalf("ClinicDayBounds() error: %v", err)
	}
	if start.Location().String() != ClinicTimezone {
		t.Fatalf("day start location = %s, want %s", start.Location(), ClinicTimezone)
	}
	if end.Sub(start) != 24*time.Hour {
		t.Fatalf("day span = %s, want 24h", end.Sub(start))
	}
}

func TestBusyDurationMinutes(t *testing.T) {
	if got := BusyDurationMinutes(45); got != 45 {
		t.Fatalf("BusyDurationMinutes(45) = %d, want 45", got)
	}
	if got := BusyDurationMinutes(0); got != DefaultBusyDurationMinutes {
		t.Fatalf("BusyDurationMinutes(0) = %d, want %d", got, DefaultBusyDurationMinutes)
	}
}
