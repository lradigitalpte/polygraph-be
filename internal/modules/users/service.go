package users

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"my-app/internal/database"
	"my-app/internal/models"
	"my-app/internal/modules/auth"
	"my-app/internal/modules/rbac"
)

type Service struct {
	db              *gorm.DB
	mu              sync.RWMutex
	usersCache      []auth.User
	usersCacheUntil time.Time
}

type CreateUserInput struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	RoleID uint   `json:"role_id"`
}

type UpdateRoleInput struct {
	RoleID uint `json:"role_id"`
}

type UpdateStatusInput struct {
	Status string `json:"status"`
}

func NewService() *Service {
	return &Service{db: database.GetDB()}
}

func (s *Service) GetAll() ([]auth.User, error) {
	s.mu.RLock()
	if time.Now().Before(s.usersCacheUntil) && s.usersCache != nil {
		cached := append([]auth.User(nil), s.usersCache...)
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	var users []auth.User
	err := s.db.Joins("Role").Order("users.created_at DESC").Find(&users).Error
	if err == nil {
		s.mu.Lock()
		s.usersCache = append([]auth.User(nil), users...)
		s.usersCacheUntil = time.Now().Add(5 * time.Second)
		s.mu.Unlock()
	}
	return users, err
}

func (s *Service) GetExaminers(search string) ([]auth.User, error) {
	query := s.db.Preload("Role").Joins("JOIN roles ON users.role_id = roles.id").Where("roles.name = ?", "Examiner").Where("LOWER(users.status) IN ?", []string{"active", "pending"})
	if trimmed := strings.TrimSpace(strings.ToLower(search)); trimmed != "" {
		like := "%" + trimmed + "%"
		query = query.Where("LOWER(users.name) LIKE ? OR LOWER(users.email) LIKE ?", like, like)
	}

	var users []auth.User
	err := query.Order("users.name ASC").Find(&users).Error
	return users, err
}

func (s *Service) GetByID(id uint) (*auth.User, error) {
	var user auth.User
	if err := s.db.Joins("Role").First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) Create(input CreateUserInput) (*auth.User, error) {
	name := strings.TrimSpace(input.Name)
	email := strings.TrimSpace(strings.ToLower(input.Email))
	if name == "" || email == "" || input.RoleID == 0 {
		return nil, errors.New("name, email, and role_id are required")
	}

	var role rbac.Role
	if err := s.db.First(&role, input.RoleID).Error; err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}

	now := time.Now()
	user := auth.User{
		Name:                  name,
		Email:                 email,
		RoleID:                role.ID,
		Status:                "pending",
		InvitedAt:             &now,
		PasswordResetRequired: true,
		PasswordResetSentAt:   &now,
	}

	if err := s.db.Create(&user).Error; err != nil {
		return nil, err
	}
	s.invalidateUsersCache()

	user.Role = role
	return &user, nil
}

func (s *Service) UpdateStatus(id uint, status string) (*auth.User, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "active" && status != "pending" && status != "suspended" {
		return nil, errors.New("status must be active, pending, or suspended")
	}

	user, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{"status": status}
	if status == "suspended" {
		now := time.Now()
		updates["suspended_at"] = &now
		user.SuspendedAt = &now
	} else {
		updates["suspended_at"] = nil
		user.SuspendedAt = nil
	}

	if err := s.db.Model(&auth.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	s.invalidateUsersCache()

	user.Status = status
	return user, nil
}

func (s *Service) UpdateRole(id uint, roleID uint) (*auth.User, error) {
	if roleID == 0 {
		return nil, errors.New("role_id is required")
	}

	var role rbac.Role
	if err := s.db.First(&role, roleID).Error; err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}

	if err := s.db.Model(&auth.User{}).Where("id = ?", id).Update("role_id", roleID).Error; err != nil {
		return nil, err
	}
	s.invalidateUsersCache()

	user, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	user.Role = role
	return user, nil
}

func (s *Service) RequirePasswordReset(id uint) (*auth.User, error) {
	now := time.Now()
	if err := s.db.Model(&auth.User{}).Where("id = ?", id).Updates(map[string]interface{}{
		"password_reset_required": true,
		"password_reset_sent_at":  &now,
	}).Error; err != nil {
		return nil, err
	}
	s.invalidateUsersCache()

	return s.GetByID(id)
}

func (s *Service) GetActivity(id uint, limit int) ([]models.AuditLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	var logs []models.AuditLog
	err := s.db.Where("user_id = ?", id).Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

func (s *Service) invalidateUsersCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usersCache = nil
	s.usersCacheUntil = time.Time{}
}
