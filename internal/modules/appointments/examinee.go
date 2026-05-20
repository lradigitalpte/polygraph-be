package appointments

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"my-app/internal/modules/subjects"

	"gorm.io/gorm"
)

// ExamineeRosterEntry is one examinee linked to a client account.
type ExamineeRosterEntry struct {
	Subject          subjects.Subject `json:"subject"`
	SessionCount     int              `json:"session_count"`
	CompletedCount   int              `json:"completed_count"`
	LastScheduledAt  *time.Time       `json:"last_scheduled_at,omitempty"`
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
		count      int
		completed  int
		last       time.Time
		hasLast    bool
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
