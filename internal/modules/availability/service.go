package availability

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"my-app/internal/database"
	"my-app/internal/modules/auth"
)

type Service struct {
	db *gorm.DB
}

func NewService() *Service {
	return &Service{db: database.GetDB()}
}

func (s *Service) ListBlocks(examinerID uint, date string) ([]Block, error) {
	query := s.db.Order("date ASC, start_time ASC")
	if examinerID > 0 {
		query = query.Where("examiner_id = ?", examinerID)
	}
	if date != "" {
		parsedDate, err := time.Parse("2006-01-02", date)
		if err != nil {
			return nil, errors.New("date must use YYYY-MM-DD")
		}
		start := parsedDate.UTC().Truncate(24 * time.Hour)
		end := start.Add(24 * time.Hour)
		query = query.Where("date >= ? AND date < ?", start, end)
	}

	var blocks []Block
	err := query.Find(&blocks).Error
	return blocks, err
}

func (s *Service) CreateBlock(block *Block) error {
	if err := s.normalizeAndValidateBlock(block); err != nil {
		return err
	}

	var examiner auth.User
	if err := s.db.First(&examiner, block.ExaminerID).Error; err != nil {
		return errors.New("examiner not found")
	}

	if err := s.ensureNoAppointmentConflict(block); err != nil {
		return err
	}

	return s.db.Create(block).Error
}

// ensureNoAppointmentConflict rejects a block that would cover an exam the examiner
// already has booked — you can't block time you're already committed to.
func (s *Service) ensureNoAppointmentConflict(block *Block) error {
	dayStart := block.Date.UTC().Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)

	type apptRow struct {
		ScheduledAt time.Time
		Duration    int
	}
	var rows []apptRow
	if err := s.db.Table("appointments").
		Select("scheduled_at, duration").
		Where("examiner_id = ? AND status <> ? AND scheduled_at >= ? AND scheduled_at < ?",
			block.ExaminerID, "cancelled", dayStart, dayEnd).
		Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	if block.IsFullDay {
		return errors.New("cannot block this day — you have exam(s) scheduled. Reschedule or cancel them first")
	}

	blockStart := clockMinutes(block.StartTime)
	blockEnd := clockMinutes(block.EndTime)
	for _, r := range rows {
		if r.Duration <= 0 {
			r.Duration = 150
		}
		start := r.ScheduledAt.UTC()
		apptStart := start.Hour()*60 + start.Minute()
		apptEnd := apptStart + r.Duration
		if apptStart < blockEnd && apptEnd > blockStart {
			return errors.New("this time range overlaps an exam you have booked — choose a different time")
		}
	}
	return nil
}

func clockMinutes(hhmm string) int {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return 0
	}
	return t.Hour()*60 + t.Minute()
}

func (s *Service) UpdateBlock(id uint, updates map[string]interface{}) (*Block, error) {
	var block Block
	if err := s.db.First(&block, id).Error; err != nil {
		return nil, err
	}

	if dateValue, ok := updates["date"].(string); ok && dateValue != "" {
		parsedDate, err := time.Parse("2006-01-02", dateValue)
		if err != nil {
			return nil, errors.New("date must use YYYY-MM-DD")
		}
		updates["date"] = parsedDate.UTC().Truncate(24 * time.Hour)
	}
	if reason, ok := updates["reason"].(string); ok {
		updates["reason"] = strings.TrimSpace(reason)
	}
	if startTime, ok := updates["start_time"].(string); ok {
		updates["start_time"] = strings.TrimSpace(startTime)
	}
	if endTime, ok := updates["end_time"].(string); ok {
		updates["end_time"] = strings.TrimSpace(endTime)
	}

	merged := block
	if examinerID, ok := updates["examiner_id"].(float64); ok {
		merged.ExaminerID = uint(examinerID)
	}
	if date, ok := updates["date"].(time.Time); ok {
		merged.Date = date
	}
	if startTime, ok := updates["start_time"].(string); ok {
		merged.StartTime = startTime
	}
	if endTime, ok := updates["end_time"].(string); ok {
		merged.EndTime = endTime
	}
	if isFullDay, ok := updates["is_full_day"].(bool); ok {
		merged.IsFullDay = isFullDay
	}
	if reason, ok := updates["reason"].(string); ok {
		merged.Reason = reason
	}

	if err := s.normalizeAndValidateBlock(&merged); err != nil {
		return nil, err
	}
	if err := s.ensureNoAppointmentConflict(&merged); err != nil {
		return nil, err
	}
	if err := s.db.Model(&block).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.db.First(&block, id).Error; err != nil {
		return nil, err
	}
	return &block, nil
}

func (s *Service) DeleteBlock(id uint) error {
	return s.db.Delete(&Block{}, id).Error
}

func (s *Service) normalizeAndValidateBlock(block *Block) error {
	if block.ExaminerID == 0 {
		return errors.New("examiner_id is required")
	}
	if block.Date.IsZero() {
		return errors.New("date is required")
	}
	block.Date = block.Date.UTC().Truncate(24 * time.Hour)
	block.Reason = strings.TrimSpace(block.Reason)
	block.StartTime = strings.TrimSpace(block.StartTime)
	block.EndTime = strings.TrimSpace(block.EndTime)

	if block.IsFullDay {
		block.StartTime = ""
		block.EndTime = ""
		return nil
	}
	if block.StartTime == "" || block.EndTime == "" {
		return errors.New("start_time and end_time are required for partial-day blocks")
	}
	if _, err := time.Parse("15:04", block.StartTime); err != nil {
		return errors.New("start_time must use HH:MM")
	}
	if _, err := time.Parse("15:04", block.EndTime); err != nil {
		return errors.New("end_time must use HH:MM")
	}
	if block.EndTime <= block.StartTime {
		return errors.New("end_time must be later than start_time")
	}
	return nil
}
