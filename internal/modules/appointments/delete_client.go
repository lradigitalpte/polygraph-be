package appointments

import (
	"errors"
	"strconv"
	"strings"

	"my-app/internal/modules/exams"
	"my-app/internal/modules/subjects"

	"gorm.io/gorm"
)

// DeleteClient removes a client and all related forensic records (examinees, sessions, documents, forms).
func (s *Service) DeleteClient(id string, confirmName string) error {
	client, err := s.GetClientByID(id)
	if err != nil {
		return err
	}
	if confirmName != "" && strings.TrimSpace(confirmName) != strings.TrimSpace(client.Name) {
		return errors.New("confirmation name does not match client name")
	}

	cid, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return errors.New("invalid client id")
	}
	clientID := uint(cid)

	return s.db.Transaction(func(tx *gorm.DB) error {
		var subjectIDs []uint
		if err := tx.Model(&subjects.Subject{}).Where("client_id = ?", clientID).Pluck("id", &subjectIDs).Error; err != nil {
			return err
		}

		if err := tx.Exec("DELETE FROM form_requests WHERE client_id = ?", clientID).Error; err != nil {
			return err
		}
		if err := tx.Where("client_id = ?", clientID).Delete(&SubjectDocument{}).Error; err != nil {
			return err
		}
		if len(subjectIDs) > 0 {
			if err := tx.Where("subject_id IN ?", subjectIDs).Delete(&SubjectDocument{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("client_id = ?", clientID).Delete(&ClientDocument{}).Error; err != nil {
			return err
		}

		if err := tx.Where("client_id = ?", clientID).Delete(&Appointment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("client_id = ?", clientID).Delete(&Quotation{}).Error; err != nil {
			return err
		}

		if err := deleteExamsForClient(tx, clientID); err != nil {
			return err
		}

		if err := tx.Where("client_id = ?", clientID).Delete(&subjects.Subject{}).Error; err != nil {
			return err
		}

		return tx.Delete(client).Error
	})
}

func deleteExamsForClient(tx *gorm.DB, clientID uint) error {
	var examIDs []uint
	if err := tx.Model(&exams.Exam{}).Where("client_id = ?", clientID).Pluck("id", &examIDs).Error; err != nil {
		return err
	}
	if len(examIDs) == 0 {
		return nil
	}

	childTables := []interface{}{
		&exams.Document{},
		&exams.ExamReport{},
		&exams.ExamQuestion{},
		&exams.ExamPhase{},
		&exams.ClinicalAssessment{},
		&exams.CaseReferral{},
	}
	for _, table := range childTables {
		if err := tx.Where("exam_id IN ?", examIDs).Delete(table).Error; err != nil {
			return err
		}
	}
	return tx.Where("id IN ?", examIDs).Delete(&exams.Exam{}).Error
}
