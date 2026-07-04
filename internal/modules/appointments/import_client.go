package appointments

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

func sanitizeImportEmail(email string) string {
	email = strings.TrimSpace(email)
	if email == "" || !strings.Contains(email, "@") {
		return ""
	}
	return truncate(email, 255)
}

func (s *Service) findOrCreateIndividualClient(tx *gorm.DB, first, last, phone, email, gender string) (uint, error) {
	first = truncate(first, 100)
	last = truncate(last, 100)
	if first == "" {
		first = "Examinee"
	}
	if last == "" {
		last = "Record"
	}
	name := strings.TrimSpace(first + " " + last)

	var client Client
	err := tx.Where("name = ? AND client_type = ?", name, "Individual").First(&client).Error
	if err == nil {
		updates := map[string]interface{}{}
		if client.Phone == "" && phone != "" {
			updates["phone"] = truncate(phone, 50)
		}
		if client.Gender == "" && gender != "" {
			updates["gender"] = truncate(gender, 20)
		}
		if client.Email == "" && email != "" {
			updates["email"] = sanitizeImportEmail(email)
		}
		if len(updates) > 0 {
			_ = tx.Model(&client).Updates(updates).Error
		}
		return client.ID, nil
	}

	cleanEmail := sanitizeImportEmail(email)
	if cleanEmail == "" {
		cleanEmail = fmt.Sprintf("import+%d@import.local", time.Now().UnixNano())
	}

	client = Client{
		Name:       name,
		ClientType: "Individual",
		Phone:      truncate(phone, 50),
		Email:      cleanEmail,
		Gender:     truncate(gender, 20),
	}
	if err := tx.Create(&client).Error; err != nil {
		return 0, err
	}
	return client.ID, nil
}

func resolveHistoricalAppointmentStatus(rawStatus string) (apptStatus, paymentStatus string, collected float64, fee float64) {
	apptStatus = "completed"
	paymentStatus = "Paid"
	statusLower := strings.ToLower(strings.TrimSpace(rawStatus))

	switch {
	case strings.Contains(statusLower, "no show"), strings.Contains(statusLower, "no-show"), strings.Contains(statusLower, "noshow"):
		apptStatus = "cancelled"
		paymentStatus = "Unpaid"
	case strings.Contains(statusLower, "cancel"):
		apptStatus = "cancelled"
		paymentStatus = "Unpaid"
	case strings.Contains(statusLower, "yet to take"), strings.Contains(statusLower, "scheduled"), strings.Contains(statusLower, "pending"):
		apptStatus = "pending"
		paymentStatus = "Unpaid"
	default:
		// Completed, Failed, Re-test, etc. — session occurred or is billable history.
		apptStatus = "completed"
		paymentStatus = "Paid"
	}
	return apptStatus, paymentStatus, collected, fee
}
