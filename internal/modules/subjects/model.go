package subjects

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"my-app/internal/utils"

	"gorm.io/gorm"
)

// Subject is the person being tested (Examinee)
type Subject struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	ClientID  *uint          `gorm:"index" json:"client_id,omitempty"`
	FirstName string         `gorm:"size:100;not null" json:"first_name"`
	LastName  string         `gorm:"size:100;not null" json:"last_name"`
	// Legacy plaintext column kept for backward compatibility with existing DB schema.
	LegacyIDNumber string `gorm:"column:id_number;size:255;not null;default:''" json:"-"`
	// These fields won't be stored in plaintext in the DB
	DateOfBirth *time.Time `gorm:"-" json:"dob"`
	IDNumber    string     `gorm:"-" json:"id_number"`
	// Hash of IDNumber for searching/unique constraints (since you can't search encrypted data easily)
	IDNumberHash *string `gorm:"size:64;uniqueIndex" json:"-"`
	// Standard fields
	Email           string `gorm:"size:255" json:"email,omitempty"`
	Phone           string `gorm:"size:50" json:"phone,omitempty"`
	EmployeeRef     string `gorm:"size:100" json:"employee_ref,omitempty"`
	Gender          string `gorm:"size:20" json:"gender"`
	Nationality     string `gorm:"size:100" json:"nationality"`
	SpokenLanguage  string `gorm:"size:100" json:"spoken_language"`
	WrittenLanguage string `gorm:"size:100" json:"written_language"`
	// EnglishProficiency captures how well the examinee speaks and understands English
	// (Native, Fluent, Conversational, Basic, None) so the examiner knows whether the
	// test can be conducted in English. InterpreterRequired flags when a translator is needed.
	EnglishProficiency  string `gorm:"size:50" json:"english_proficiency"`
	InterpreterRequired bool   `gorm:"default:false" json:"interpreter_required"`
	// Encrypted storage for PII (DOB, IDNumber)
	EncryptedDetails string `gorm:"type:text;not null" json:"-"`
}

type subjectPII struct {
	IDNumber    string     `json:"id_number"`
	DateOfBirth *time.Time `json:"dob"`
}

// BeforeSave encrypts the PII fields before writing to DB
func (s *Subject) BeforeSave(tx *gorm.DB) (err error) {
	pii := subjectPII{
		IDNumber:    s.IDNumber,
		DateOfBirth: s.DateOfBirth,
	}

	bytes, err := json.Marshal(pii)
	if err != nil {
		return err
	}

	encrypted, err := utils.Encrypt(string(bytes))
	if err != nil {
		return err
	}

	s.EncryptedDetails = encrypted

	// Create Blind Index for IDNumber; leave as NULL when empty so the unique index allows multiple blank IDs.
	if trimmedIDNumber := strings.TrimSpace(s.IDNumber); trimmedIDNumber != "" {
		hash := sha256.Sum256([]byte(trimmedIDNumber))
		hashString := hex.EncodeToString(hash[:])
		s.IDNumberHash = &hashString
	} else {
		s.IDNumberHash = nil
	}

	// Keep legacy non-null column populated to avoid schema-compat insert failures.
	s.LegacyIDNumber = s.IDNumber

	return nil
}

// AfterFind decrypts the PII fields after reading from DB
func (s *Subject) AfterFind(tx *gorm.DB) (err error) {
	if s.EncryptedDetails == "" {
		return nil
	}

	decryptedStr, err := utils.Decrypt(s.EncryptedDetails)
	if err != nil {
		return err
	}

	var pii subjectPII
	if err := json.Unmarshal([]byte(decryptedStr), &pii); err != nil {
		return err
	}

	s.IDNumber = pii.IDNumber
	s.DateOfBirth = pii.DateOfBirth
	return nil
}
