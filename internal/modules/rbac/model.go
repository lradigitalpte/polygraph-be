package rbac


// Permission defines a specific action a user can perform
type Permission struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	Name        string    `gorm:"size:100;uniqueIndex;not null" json:"name"` // e.g. "exam:create"
	Description string    `gorm:"size:255" json:"description"`
	Group       string    `gorm:"size:100" json:"group"` // e.g. "Exams", "Users"
}

// Role is a collection of permissions
type Role struct {
	ID          uint         `gorm:"primarykey" json:"id"`
	Name        string       `gorm:"size:100;uniqueIndex;not null" json:"name"`
	Description string       `gorm:"size:255" json:"description"`
	Permissions []Permission `gorm:"many2many:role_permissions;" json:"permissions,omitempty"`
}

// UserPermission is a per-user override that takes precedence over the user's role.
// Granted=true explicitly grants a permission the role lacks; Granted=false explicitly
// denies a permission the role would otherwise allow.
type UserPermission struct {
	UserID       uint `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	PermissionID uint `gorm:"primaryKey;autoIncrement:false" json:"permission_id"`
	Granted      bool `json:"granted"`
}
