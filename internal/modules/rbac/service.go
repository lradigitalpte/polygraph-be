package rbac

import (
	"errors"
	"strings"
	"sync"
	"time"

	"my-app/internal/database"

	"gorm.io/gorm"
)

type Service struct {
	db                    *gorm.DB
	mu                    sync.RWMutex
	permissionsCache      []Permission
	permissionsCacheUntil time.Time
	rolesCache            []Role
	rolesCacheUntil       time.Time
	permissionsLoading    bool
	permissionsWaiters    []chan permissionsResult
	rolesLoading          bool
	rolesWaiters          []chan rolesResult
}

type permissionsResult struct {
	permissions []Permission
	err         error
}

type rolesResult struct {
	roles []Role
	err   error
}

const rbacCacheTTL = 10 * time.Second

func NewService() *Service {
	return &Service{db: database.GetDB()}
}

func (s *Service) GetAllPermissions() ([]Permission, error) {
	s.mu.RLock()
	if time.Now().Before(s.permissionsCacheUntil) && s.permissionsCache != nil {
		cached := append([]Permission(nil), s.permissionsCache...)
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	if time.Now().Before(s.permissionsCacheUntil) && s.permissionsCache != nil {
		cached := append([]Permission(nil), s.permissionsCache...)
		s.mu.Unlock()
		return cached, nil
	}
	if s.permissionsLoading {
		ch := make(chan permissionsResult, 1)
		s.permissionsWaiters = append(s.permissionsWaiters, ch)
		s.mu.Unlock()
		result := <-ch
		return result.permissions, result.err
	}
	s.permissionsLoading = true
	s.mu.Unlock()

	var permissions []Permission
	err := s.db.Find(&permissions).Error
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.permissionsCache = append([]Permission(nil), permissions...)
		s.permissionsCacheUntil = time.Now().Add(rbacCacheTTL)
	}
	for _, waiter := range s.permissionsWaiters {
		waiter <- permissionsResult{permissions: append([]Permission(nil), permissions...), err: err}
		close(waiter)
	}
	s.permissionsWaiters = nil
	s.permissionsLoading = false
	return permissions, err
}

func (s *Service) GetAllRoles() ([]Role, error) {
	s.mu.RLock()
	if time.Now().Before(s.rolesCacheUntil) && s.rolesCache != nil {
		cached := append([]Role(nil), s.rolesCache...)
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	if time.Now().Before(s.rolesCacheUntil) && s.rolesCache != nil {
		cached := append([]Role(nil), s.rolesCache...)
		s.mu.Unlock()
		return cached, nil
	}
	if s.rolesLoading {
		ch := make(chan rolesResult, 1)
		s.rolesWaiters = append(s.rolesWaiters, ch)
		s.mu.Unlock()
		result := <-ch
		return result.roles, result.err
	}
	s.rolesLoading = true
	s.mu.Unlock()

	var roles []Role
	err := s.db.Preload("Permissions").Find(&roles).Error
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.rolesCache = append([]Role(nil), roles...)
		s.rolesCacheUntil = time.Now().Add(rbacCacheTTL)
	}
	for _, waiter := range s.rolesWaiters {
		waiter <- rolesResult{roles: append([]Role(nil), roles...), err: err}
		close(waiter)
	}
	s.rolesWaiters = nil
	s.rolesLoading = false
	return roles, err
}

// UpdateRole updates a role's name/description and/or replaces its permission set.
// Any nil field is left unchanged.
func (s *Service) UpdateRole(id uint, name, description *string, permissionIDs *[]uint) (*Role, error) {
	var role Role
	if err := s.db.First(&role, id).Error; err != nil {
		return nil, errors.New("role not found")
	}

	updates := map[string]interface{}{}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return nil, errors.New("role name cannot be empty")
		}
		updates["name"] = trimmed
	}
	if description != nil {
		updates["description"] = strings.TrimSpace(*description)
	}
	if len(updates) > 0 {
		if err := s.db.Model(&role).Updates(updates).Error; err != nil {
			return nil, err
		}
	}

	if permissionIDs != nil {
		var permissions []Permission
		if len(*permissionIDs) > 0 {
			s.db.Find(&permissions, *permissionIDs)
		}
		if err := s.db.Model(&role).Association("Permissions").Replace(permissions); err != nil {
			return nil, err
		}
	}

	s.invalidateCaches()

	var updated Role
	if err := s.db.Preload("Permissions").First(&updated, id).Error; err != nil {
		return nil, err
	}
	return &updated, nil
}

func (s *Service) CreateRole(name, description string, permissionIDs []uint) (*Role, error) {
	var permissions []Permission
	if len(permissionIDs) > 0 {
		s.db.Find(&permissions, permissionIDs)
	}

	role := Role{
		Name:        name,
		Description: description,
		Permissions: permissions,
	}

	err := s.db.Create(&role).Error
	if err == nil {
		s.invalidateCaches()
	}
	return &role, err
}

func (s *Service) invalidateCaches() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rolesCache = nil
	s.permissionsCache = nil
	s.rolesCacheUntil = time.Time{}
	s.permissionsCacheUntil = time.Time{}
	s.rolesWaiters = nil
	s.permissionsWaiters = nil
	s.rolesLoading = false
	s.permissionsLoading = false
}
