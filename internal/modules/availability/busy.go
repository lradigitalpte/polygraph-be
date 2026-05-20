package availability

import (
	"errors"
	"fmt"
	"time"
)

// BusyPeriod is a time range when the examiner cannot be booked (manual block or existing appointment).
type BusyPeriod struct {
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	Source     string `json:"source"` // block, appointment
	Reason     string `json:"reason,omitempty"`
	IsFullDay  bool   `json:"is_full_day,omitempty"`
}

type appointmentBusyRow struct {
	ScheduledAt time.Time `gorm:"column:scheduled_at"`
	Duration    int       `gorm:"column:duration"`
}

func (s *Service) GetExaminerDaySchedule(examinerID uint, date string) (blocks []Block, busy []BusyPeriod, isBlocked bool, err error) {
	blocks, err = s.ListBlocks(examinerID, date)
	if err != nil {
		return nil, nil, false, err
	}

	for _, block := range blocks {
		if block.IsFullDay {
			isBlocked = true
		}
		busy = append(busy, blockToBusyPeriod(block))
	}

	appointments, apptErr := s.listExaminerAppointmentsForDay(examinerID, date)
	if apptErr != nil {
		return nil, nil, false, apptErr
	}
	busy = append(busy, appointments...)

	return blocks, busy, isBlocked, nil
}

func blockToBusyPeriod(block Block) BusyPeriod {
	period := BusyPeriod{
		Source:    "block",
		Reason:    block.Reason,
		IsFullDay: block.IsFullDay,
	}
	if block.IsFullDay {
		return period
	}
	period.StartTime = block.StartTime
	period.EndTime = block.EndTime
	return period
}

func (s *Service) listExaminerAppointmentsForDay(examinerID uint, date string) ([]BusyPeriod, error) {
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, errors.New("date must use YYYY-MM-DD")
	}
	dayStart := parsedDate.UTC().Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)

	var rows []appointmentBusyRow
	err = s.db.Table("appointments").
		Select("scheduled_at", "duration").
		Where("examiner_id = ? AND status <> ? AND scheduled_at >= ? AND scheduled_at < ?", examinerID, "cancelled", dayStart, dayEnd).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	periods := make([]BusyPeriod, 0, len(rows))
	for _, row := range rows {
		if row.Duration <= 0 {
			row.Duration = 150
		}
		start := row.ScheduledAt.UTC()
		end := start.Add(time.Duration(row.Duration) * time.Minute)
		periods = append(periods, BusyPeriod{
			StartTime: formatClock(start),
			EndTime:   formatClock(end),
			Source:    "appointment",
			Reason:    "Booked session",
		})
	}
	return periods, nil
}

func formatClock(t time.Time) string {
	return fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute())
}
