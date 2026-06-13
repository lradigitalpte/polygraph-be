package appointments

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"my-app/internal/modules/subjects"

	"gorm.io/gorm"
)

// ExamineeRosterEntry is one examinee linked to a client account.
type ExamineeRosterEntry struct {
	Subject         subjects.Subject `json:"subject"`
	SessionCount    int              `json:"session_count"`
	CompletedCount  int              `json:"completed_count"`
	LastScheduledAt *time.Time       `json:"last_scheduled_at,omitempty"`
}

func (s *Service) GetClientExaminees(clientID string) ([]ExamineeRosterEntry, error) {
	if _, err := s.GetClientByID(clientID); err != nil {
		return nil, err
	}

	cid, err := strconv.ParseUint(clientID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid client id")
	}

	var appointments []Appointment
	if err := s.db.Where("client_id = ?", cid).Order("scheduled_at DESC").Find(&appointments).Error; err != nil {
		return nil, err
	}

	stats := make(map[uint]struct {
		count     int
		completed int
		last      time.Time
		hasLast   bool
	})
	for _, appt := range appointments {
		st := stats[appt.SubjectID]
		st.count++
		if appt.Status == "completed" {
			st.completed++
		}
		if !st.hasLast || appt.ScheduledAt.After(st.last) {
			st.last = appt.ScheduledAt
			st.hasLast = true
		}
		stats[appt.SubjectID] = st
	}

	seen := make(map[uint]bool)
	orderedIDs := make([]uint, 0, len(stats))
	for _, appt := range appointments {
		if seen[appt.SubjectID] {
			continue
		}
		seen[appt.SubjectID] = true
		orderedIDs = append(orderedIDs, appt.SubjectID)
	}

	var rosterSubjects []subjects.Subject
	if err := s.db.Where("client_id = ?", cid).Order("first_name ASC, last_name ASC").Find(&rosterSubjects).Error; err != nil {
		return nil, err
	}
	for _, subj := range rosterSubjects {
		if seen[subj.ID] {
			continue
		}
		seen[subj.ID] = true
		orderedIDs = append(orderedIDs, subj.ID)
	}

	if len(orderedIDs) == 0 {
		return []ExamineeRosterEntry{}, nil
	}

	var subjectRows []subjects.Subject
	if err := s.db.Where("id IN ?", orderedIDs).Find(&subjectRows).Error; err != nil {
		return nil, err
	}
	byID := make(map[uint]subjects.Subject, len(subjectRows))
	for _, subj := range subjectRows {
		byID[subj.ID] = subj
	}

	entries := make([]ExamineeRosterEntry, 0, len(orderedIDs))
	for _, sid := range orderedIDs {
		subj, ok := byID[sid]
		if !ok {
			continue
		}
		st := stats[sid]
		var last *time.Time
		if st.hasLast {
			t := st.last
			last = &t
		}
		entries = append(entries, ExamineeRosterEntry{
			Subject:         subj,
			SessionCount:    st.count,
			CompletedCount:  st.completed,
			LastScheduledAt: last,
		})
	}

	return entries, nil
}

func (s *Service) GetSubjectAppointments(subjectID, clientID string) ([]Appointment, error) {
	sid, err := strconv.ParseUint(subjectID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid subject id")
	}

	var subject subjects.Subject
	if err := s.db.First(&subject, sid).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("subject not found")
		}
		return nil, err
	}

	query := s.db.Preload("Client").Where("subject_id = ?", sid).Order("scheduled_at DESC")
	if trimmed := clientID; trimmed != "" {
		query = query.Where("client_id = ?", trimmed)
	}

	var list []Appointment
	if err := query.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

type bulkExamineeInput struct {
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	EmployeeRef string `json:"employee_ref"`
}

func (s *Service) BulkCreateExaminees(clientID string, inputs []bulkExamineeInput) ([]subjects.Subject, error) {
	if _, err := s.GetClientByID(clientID); err != nil {
		return nil, err
	}

	cid64, err := strconv.ParseUint(clientID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid client id")
	}
	clientIDUint := uint(cid64)

	created := make([]subjects.Subject, 0, len(inputs))
	subjectSvc := subjects.NewService()

	for _, row := range inputs {
		first := strings.TrimSpace(row.FirstName)
		last := strings.TrimSpace(row.LastName)
		if first == "" && last == "" {
			continue
		}
		if first == "" {
			first = "Examinee"
		}
		if last == "" {
			last = "Record"
		}

		subj := &subjects.Subject{
			ClientID:    &clientIDUint,
			FirstName:   first,
			LastName:    last,
			Email:       strings.TrimSpace(row.Email),
			Phone:       strings.TrimSpace(row.Phone),
			EmployeeRef: strings.TrimSpace(row.EmployeeRef),
		}
		if err := subjectSvc.Create(subj); err != nil {
			return created, err
		}
		created = append(created, *subj)
	}

	if len(created) == 0 {
		return nil, errors.New("no valid examinee rows to import")
	}

	return created, nil
}

// BulkScheduleInput describes one examinee row in a batch intake request.
type BulkScheduleInput struct {
	// Existing subject — provide subject_id to skip creation.
	SubjectID *uint `json:"subject_id"`
	// New subject fields (used when subject_id is nil).
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	EmployeeRef string `json:"employee_ref"`
	// Per-examinee optional schedule offset in minutes from the batch start time.
	// If 0 all examinees share the same ScheduledAt.
	OffsetMinutes int `json:"offset_minutes"`
}

// BulkScheduleResult is the response for one examinee in the batch.
type BulkScheduleResult struct {
	Subject     subjects.Subject `json:"subject"`
	Appointment Appointment      `json:"appointment"`
}

// BulkSchedule creates subjects (if new) and one appointment per examinee in a
// single transaction. All appointments share client, examiner, duration, fee and
// payment fields; each may optionally have a per-row time offset.
func (s *Service) BulkSchedule(
	clientID uint,
	examinerID uint,
	scheduledAt time.Time,
	duration int,
	examFee float64,
	paymentMode string,
	notes string,
	rows []BulkScheduleInput,
) ([]BulkScheduleResult, error) {
	if _, err := s.GetClientByID(strconv.FormatUint(uint64(clientID), 10)); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("at least one examinee is required")
	}
	if duration <= 0 {
		duration = 60
	}

	subjectSvc := subjects.NewService()
	results := make([]BulkScheduleResult, 0, len(rows))

	err := s.db.Transaction(func(tx *gorm.DB) error {
		txSvc := &Service{db: tx, storage: s.storage}

		for i, row := range rows {
			var subj subjects.Subject

			if row.SubjectID != nil && *row.SubjectID > 0 {
				// Use existing subject.
				found, err := subjectSvc.GetByID(strconv.FormatUint(uint64(*row.SubjectID), 10))
				if err != nil {
					return fmt.Errorf("row %d: subject %d not found", i+1, *row.SubjectID)
				}
				subj = *found
			} else {
				// Create new subject.
				first := strings.TrimSpace(row.FirstName)
				last := strings.TrimSpace(row.LastName)
				if first == "" && last == "" {
					continue // skip blank rows silently
				}
				if first == "" {
					first = "Examinee"
				}
				if last == "" {
					last = "Record"
				}
				subj = subjects.Subject{
					ClientID:    &clientID,
					FirstName:   first,
					LastName:    last,
					Email:       strings.TrimSpace(row.Email),
					Phone:       strings.TrimSpace(row.Phone),
					EmployeeRef: strings.TrimSpace(row.EmployeeRef),
				}
				if err := tx.Create(&subj).Error; err != nil {
					return fmt.Errorf("row %d: failed to create subject: %w", i+1, err)
				}
			}

			apptTime := scheduledAt.Add(time.Duration(row.OffsetMinutes) * time.Minute)
			appt := Appointment{
				ClientID:      clientID,
				SubjectID:     subj.ID,
				ExaminerID:    examinerID,
				ScheduledAt:   apptTime,
				Duration:      duration,
				ExamFee:       examFee,
				PaymentMode:   paymentMode,
				PaymentStatus: "Unpaid",
				Status:        "pending",
				Notes:         notes,
			}
			if err := txSvc.CreateAppointment(&appt); err != nil {
				return fmt.Errorf("row %d (%s %s): %w", i+1, subj.FirstName, subj.LastName, err)
			}

			results = append(results, BulkScheduleResult{Subject: subj, Appointment: appt})
		}

		if len(results) == 0 {
			return errors.New("no valid examinee rows to schedule")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Notify each scheduled examinee (best-effort, after commit so a mail failure
	// never rolls back the batch). Examinees without an email are skipped inside.
	for _, r := range results {
		_ = s.EmailAppointmentConfirmation(r.Appointment.ID)
	}

	return results, nil
}
