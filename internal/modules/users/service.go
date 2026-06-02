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

// PermissionState is a permission plus how it resolves for a specific user.
type PermissionState struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Group       string `json:"group"`
	RoleDefault bool   `json:"role_default"` // granted by the user's role
	Override    *bool  `json:"override"`     // nil = no override, else explicit allow/deny
	Effective   bool   `json:"effective"`    // what actually applies
}

type PermissionOverrideInput struct {
	PermissionID uint `json:"permission_id"`
	Granted      bool `json:"granted"`
}

type SetPermissionsInput struct {
	Overrides []PermissionOverrideInput `json:"overrides"`
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

	// Omit the Role association — otherwise GORM overwrites role_id with the
	// zero-value association (0), violating the users.role_id FK.
	if err := s.db.Omit("Role").Create(&user).Error; err != nil {
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

// GetUserPermissions returns every permission with its role default, any per-user
// override, and the effective result for the given user.
func (s *Service) GetUserPermissions(userID uint) ([]PermissionState, error) {
	user, err := s.GetByID(userID)
	if err != nil {
		return nil, err
	}

	var allPerms []rbac.Permission
	if err := s.db.Order("\"group\" ASC, name ASC").Find(&allPerms).Error; err != nil {
		return nil, err
	}

	// Permission IDs granted by the user's role.
	roleGranted := map[uint]bool{}
	var rolePermRows []struct{ PermissionID uint }
	s.db.Table("role_permissions").Select("permission_id").Where("role_id = ?", user.RoleID).Scan(&rolePermRows)
	for _, r := range rolePermRows {
		roleGranted[r.PermissionID] = true
	}

	// Per-user overrides.
	overrides := map[uint]bool{}
	var ovrRows []rbac.UserPermission
	s.db.Where("user_id = ?", userID).Find(&ovrRows)
	for _, o := range ovrRows {
		overrides[o.PermissionID] = o.Granted
	}

	states := make([]PermissionState, 0, len(allPerms))
	for _, p := range allPerms {
		roleDefault := roleGranted[p.ID]
		state := PermissionState{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
			Group:       p.Group,
			RoleDefault: roleDefault,
			Effective:   roleDefault,
		}
		if granted, ok := overrides[p.ID]; ok {
			g := granted
			state.Override = &g
			state.Effective = granted
		}
		states = append(states, state)
	}
	return states, nil
}

// SetUserPermissions replaces all per-user overrides for a user in one transaction.
func (s *Service) SetUserPermissions(userID uint, input SetPermissionsInput) error {
	if _, err := s.GetByID(userID); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&rbac.UserPermission{}).Error; err != nil {
			return err
		}
		if len(input.Overrides) == 0 {
			return nil
		}
		rows := make([]rbac.UserPermission, 0, len(input.Overrides))
		for _, o := range input.Overrides {
			rows = append(rows, rbac.UserPermission{UserID: userID, PermissionID: o.PermissionID, Granted: o.Granted})
		}
		return tx.Create(&rows).Error
	})
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
