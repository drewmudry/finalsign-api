package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WorkspaceMembership represents the relationship between users and workspaces
type WorkspaceMembership struct {
	ID          uuid.UUID        `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	WorkspaceID uuid.UUID        `gorm:"type:uuid;not null" json:"workspace_id"`
	UserID      int              `gorm:"not null" json:"user_id"`
	Role        MembershipRole   `gorm:"type:membership_role;not null;default:'member'" json:"role"`
	Status      MembershipStatus `gorm:"type:membership_status;default:'active'" json:"status"`
	InvitedBy   *int             `gorm:"column:invited_by" json:"invited_by,omitempty"`
	InvitedAt   *time.Time       `gorm:"column:invited_at" json:"invited_at,omitempty"`
	JoinedAt    time.Time        `gorm:"column:joined_at" json:"joined_at"`
	CreatedAt   time.Time        `gorm:"column:created_at" json:"created_at"`

	// Associations
	Workspace Workspace `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
	User      User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Inviter   *User     `gorm:"foreignKey:InvitedBy" json:"inviter,omitempty"`
}

// TableName specifies the table name for the WorkspaceMembership model
func (WorkspaceMembership) TableName() string {
	return "workspace_memberships"
}

// BeforeCreate sets the joined_at timestamp if not set
func (wm *WorkspaceMembership) BeforeCreate(tx *gorm.DB) error {
	if wm.JoinedAt.IsZero() {
		wm.JoinedAt = time.Now()
	}
	return nil
}

// WorkspaceMembershipManager provides Django-like ORM methods for WorkspaceMembership
type WorkspaceMembershipManager struct {
	db *gorm.DB
}

// NewWorkspaceMembershipManager creates a new WorkspaceMembershipManager instance
func NewWorkspaceMembershipManager(db *gorm.DB) *WorkspaceMembershipManager {
	return &WorkspaceMembershipManager{db: db}
}

// Create creates a new workspace membership
func (m *WorkspaceMembershipManager) Create(membership *WorkspaceMembership) error {
	return m.db.Create(membership).Error
}

// Get retrieves a membership by ID
func (m *WorkspaceMembershipManager) Get(id uuid.UUID) (*WorkspaceMembership, error) {
	var membership WorkspaceMembership
	err := m.db.First(&membership, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &membership, nil
}

// GetByUserAndWorkspace retrieves a membership by user and workspace
func (m *WorkspaceMembershipManager) GetByUserAndWorkspace(userID int, workspaceID uuid.UUID) (*WorkspaceMembership, error) {
	var membership WorkspaceMembership
	err := m.db.Where("user_id = ? AND workspace_id = ?", userID, workspaceID).First(&membership).Error
	if err != nil {
		return nil, err
	}
	return &membership, nil
}

// Filter retrieves memberships matching the given conditions
func (m *WorkspaceMembershipManager) Filter(conditions interface{}) ([]WorkspaceMembership, error) {
	var memberships []WorkspaceMembership
	err := m.db.Where(conditions).Find(&memberships).Error
	return memberships, err
}

// Update updates a membership
func (m *WorkspaceMembershipManager) Update(membership *WorkspaceMembership) error {
	return m.db.Save(membership).Error
}

// Delete deletes a membership
func (m *WorkspaceMembershipManager) Delete(id uuid.UUID) error {
	return m.db.Delete(&WorkspaceMembership{}, "id = ?", id).Error
}

// Django-like instance methods for WorkspaceMembership

// Save saves the membership instance
func (wm *WorkspaceMembership) Save(db *gorm.DB) error {
	return db.Save(wm).Error
}

// Delete deletes the membership instance
func (wm *WorkspaceMembership) Delete(db *gorm.DB) error {
	return db.Delete(wm).Error
}

// Activate activates the membership
func (wm *WorkspaceMembership) Activate(db *gorm.DB) error {
	wm.Status = StatusActive
	return db.Save(wm).Error
}

// Suspend suspends the membership
func (wm *WorkspaceMembership) Suspend(db *gorm.DB) error {
	wm.Status = StatusSuspended
	return db.Save(wm).Error
}

// UpdateRole updates the membership role
func (wm *WorkspaceMembership) UpdateRole(db *gorm.DB, newRole MembershipRole) error {
	wm.Role = newRole
	return db.Save(wm).Error
}

// IsActive checks if the membership is active
func (wm *WorkspaceMembership) IsActive() bool {
	return wm.Status == StatusActive
}

// IsOwner checks if the membership has owner role
func (wm *WorkspaceMembership) IsOwner() bool {
	return wm.Role == RoleOwner
}

// IsAdmin checks if the membership has admin role (or owner)
func (wm *WorkspaceMembership) IsAdmin() bool {
	return wm.Role == RoleAdmin || wm.Role == RoleOwner
}

// CanManageMembers checks if the membership can manage other members
func (wm *WorkspaceMembership) CanManageMembers() bool {
	return wm.IsActive() && (wm.Role == RoleOwner || wm.Role == RoleAdmin)
}

// CanEditWorkspace checks if the membership can edit workspace settings
func (wm *WorkspaceMembership) CanEditWorkspace() bool {
	return wm.IsActive() && (wm.Role == RoleOwner || wm.Role == RoleAdmin)
}
