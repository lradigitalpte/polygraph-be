package users

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"my-app/internal/modules/auth"
)

type UpdateMeInput struct {
	Name string `json:"name"`
}

func (s *Service) GetMe(userID uint) (*auth.User, error) {
	return s.GetByID(userID)
}

func (s *Service) UpdateMe(userID uint, input UpdateMeInput) (*auth.User, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	if err := s.db.Model(&auth.User{}).Where("id = ?", userID).Update("name", name).Error; err != nil {
		return nil, err
	}
	s.invalidateUsersCache()
	return s.GetByID(userID)
}

func (s *Service) DeleteUser(id uint, actorID uint) error {
	if id == actorID {
		return errors.New("use DELETE /api/me to delete your own account")
	}

	user, err := s.GetByID(id)
	if err != nil {
		return err
	}

	return s.hardDeleteUser(user)
}

func (s *Service) DeleteMe(userID uint) error {
	user, err := s.GetByID(userID)
	if err != nil {
		return err
	}
	return s.hardDeleteUser(user)
}

func (s *Service) hardDeleteUser(user *auth.User) error {
	var remaining int64
	if err := s.db.Model(&auth.User{}).
		Where("id <> ?", user.ID).
		Where("LOWER(status) IN ?", []string{"active", "pending"}).
		Count(&remaining).Error; err != nil {
		return err
	}
	if remaining == 0 {
		return errors.New("cannot delete the last active user")
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := deleteAuthUser(tx, user.Email); err != nil {
			return fmt.Errorf("failed to remove auth account: %w", err)
		}
		if err := tx.Unscoped().Delete(&auth.User{}, user.ID).Error; err != nil {
			return err
		}
		s.invalidateUsersCache()
		return nil
	})
}

// deleteAuthUser removes the self-hosted better-auth credential (user/account/session)
// for the given email. These tables live in the public schema alongside GORM's tables.
func deleteAuthUser(tx *gorm.DB, email string) error {
	var userID string
	if err := tx.Raw(`SELECT id FROM "user" WHERE LOWER(email) = LOWER(?)`, email).Scan(&userID).Error; err != nil {
		return err
	}
	if userID == "" {
		return nil
	}
	if err := tx.Exec(`DELETE FROM session WHERE "userId" = ?`, userID).Error; err != nil {
		return err
	}
	if err := tx.Exec(`DELETE FROM account WHERE "userId" = ?`, userID).Error; err != nil {
		return err
	}
	return tx.Exec(`DELETE FROM "user" WHERE id = ?`, userID).Error
}
