package models

import (
	"crypto/rand"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// JSONB handles JSON data storage
type JSONB map[string]interface{}

// Value implements the driver.Valuer interface
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return "{}", nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = make(map[string]interface{})
		return nil
	}

	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, j)
	case string:
		return json.Unmarshal([]byte(v), j)
	default:
		return errors.New("unsupported type for JSONB")
	}
}

// Workspace represents a workspace in the system
type Workspace struct {
	ID          uuid.UUID     `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name        string        `gorm:"column:name;not null" json:"name"`
	Slug        string        `gorm:"column:slug;uniqueIndex;not null" json:"slug"`
	Description string        `gorm:"column:description" json:"description"`
	Plan        WorkspacePlan `gorm:"column:plan;default:'free'" json:"plan"`
	Settings    JSONB         `gorm:"column:settings;type:jsonb;default:'{}'" json:"settings"`
	IsActive    bool          `gorm:"column:is_active;default:true" json:"is_active"`
	CreatedAt   time.Time     `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time     `gorm:"column:updated_at" json:"updated_at"`

	// Associations
	Memberships []WorkspaceMembership `gorm:"foreignKey:WorkspaceID" json:"memberships,omitempty"`
	Members     []User                `gorm:"many2many:workspace_memberships;" json:"members,omitempty"`
}

// TableName specifies the table name for the Workspace model
func (Workspace) TableName() string {
	return "workspaces"
}

// BeforeCreate generates a unique slug if not provided
func (w *Workspace) BeforeCreate(tx *gorm.DB) error {
	if w.Slug == "" {
		// Generate a 6-character alphanumeric slug
		for attempts := 0; attempts < 100; attempts++ {
			slug := generateSlug(6)
			var count int64
			tx.Model(&Workspace{}).Where("slug = ?", slug).Count(&count)
			if count == 0 {
				w.Slug = slug
				break
			}
		}
		if w.Slug == "" {
			return errors.New("could not generate unique slug")
		}
	}
	return nil
}

// generateSlug generates a random alphanumeric string of given length
func generateSlug(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	rand.Read(b)
	for i := range b {
		b[i] = charset[b[i]%byte(len(charset))]
	}
	return string(b)
}

// WorkspaceManager provides Django-like ORM methods for Workspace
type WorkspaceManager struct {
	db *gorm.DB
}

// NewWorkspaceManager creates a new WorkspaceManager instance
func NewWorkspaceManager(db *gorm.DB) *WorkspaceManager {
	return &WorkspaceManager{db: db}
}

// Create creates a new workspace
func (m *WorkspaceManager) Create(workspace *Workspace) error {
	return m.db.Create(workspace).Error
}

// Get retrieves a workspace by ID
func (m *WorkspaceManager) Get(id uuid.UUID) (*Workspace, error) {
	var workspace Workspace
	err := m.db.First(&workspace, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &workspace, nil
}

// GetBySlug retrieves a workspace by slug
func (m *WorkspaceManager) GetBySlug(slug string) (*Workspace, error) {
	var workspace Workspace
	err := m.db.Where("slug = ? AND is_active = ?", slug, true).First(&workspace).Error
	if err != nil {
		return nil, err
	}
	return &workspace, nil
}

// All retrieves all workspaces
func (m *WorkspaceManager) All() ([]Workspace, error) {
	var workspaces []Workspace
	err := m.db.Find(&workspaces).Error
	return workspaces, err
}

// Filter retrieves workspaces matching the given conditions
func (m *WorkspaceManager) Filter(conditions interface{}) ([]Workspace, error) {
	var workspaces []Workspace
	err := m.db.Where(conditions).Find(&workspaces).Error
	return workspaces, err
}

// Update updates a workspace
func (m *WorkspaceManager) Update(workspace *Workspace) error {
	return m.db.Save(workspace).Error
}

// Delete soft deletes a workspace
func (m *WorkspaceManager) Delete(id uuid.UUID) error {
	return m.db.Delete(&Workspace{}, "id = ?", id).Error
}

// Django-like instance methods for Workspace

// Save saves the workspace instance
func (w *Workspace) Save(db *gorm.DB) error {
	return db.Save(w).Error
}

// Delete soft deletes the workspace instance
func (w *Workspace) Delete(db *gorm.DB) error {
	return db.Delete(w).Error
}

// AddMember adds a user as a member to the workspace (Django-style)
func (w *Workspace) AddMember(db *gorm.DB, user *User, role MembershipRole) error {
	// Check if user is already a member
	var existingMembership WorkspaceMembership
	err := db.Where("workspace_id = ? AND user_id = ?", w.ID, user.ID).First(&existingMembership).Error
	if err == nil {
		// Update existing membership
		existingMembership.Status = StatusActive
		existingMembership.Role = role
		return db.Save(&existingMembership).Error
	}

	// Create new membership
	membership := &WorkspaceMembership{
		WorkspaceID: w.ID,
		UserID:      user.ID,
		Role:        role,
		Status:      StatusActive,
		JoinedAt:    time.Now(),
	}

	return db.Create(membership).Error
}

// RemoveMember removes a user from the workspace
func (w *Workspace) RemoveMember(db *gorm.DB, user *User) error {
	// Check if this is the last owner
	if err := w.validateOwnerRemoval(db, user.ID); err != nil {
		return err
	}

	return db.Where("workspace_id = ? AND user_id = ?", w.ID, user.ID).
		Delete(&WorkspaceMembership{}).Error
}

// UpdateMemberRole updates a member's role in the workspace
func (w *Workspace) UpdateMemberRole(db *gorm.DB, user *User, newRole MembershipRole) error {
	// Check if changing from owner and validate
	var membership WorkspaceMembership
	err := db.Where("workspace_id = ? AND user_id = ?", w.ID, user.ID).First(&membership).Error
	if err != nil {
		return fmt.Errorf("user is not a member of this workspace")
	}

	if membership.Role == RoleOwner && newRole != RoleOwner {
		if err := w.validateOwnerRemoval(db, user.ID); err != nil {
			return err
		}
	}

	membership.Role = newRole
	return db.Save(&membership).Error
}

// GetMembers retrieves all members of the workspace
func (w *Workspace) GetMembers(db *gorm.DB) ([]User, error) {
	var users []User
	err := db.Joins("JOIN workspace_memberships ON workspace_memberships.user_id = users.id").
		Where("workspace_memberships.workspace_id = ? AND workspace_memberships.status = ?", w.ID, StatusActive).
		Find(&users).Error
	return users, err
}

// GetMembersWithRoles retrieves all members with their roles
func (w *Workspace) GetMembersWithRoles(db *gorm.DB) ([]WorkspaceMember, error) {
	var members []WorkspaceMember
	query := `
		SELECT u.id, u.email, u.name, u.avatar_url, 
			   wm.role, wm.status, wm.joined_at
		FROM users u
		JOIN workspace_memberships wm ON u.id = wm.user_id
		WHERE wm.workspace_id = ? AND wm.status = ?
		ORDER BY wm.joined_at ASC
	`
	err := db.Raw(query, w.ID, StatusActive).Scan(&members).Error
	return members, err
}

// HasMember checks if a user is a member of the workspace
func (w *Workspace) HasMember(db *gorm.DB, userID int) (bool, error) {
	var count int64
	err := db.Model(&WorkspaceMembership{}).
		Where("workspace_id = ? AND user_id = ? AND status = ?", w.ID, userID, StatusActive).
		Count(&count).Error
	return count > 0, err
}

// GetMemberRole gets a user's role in the workspace
func (w *Workspace) GetMemberRole(db *gorm.DB, userID int) (MembershipRole, error) {
	var membership WorkspaceMembership
	err := db.Where("workspace_id = ? AND user_id = ? AND status = ?", w.ID, userID, StatusActive).
		First(&membership).Error
	if err != nil {
		return "", err
	}
	return membership.Role, nil
}

// IsOwner checks if a user is an owner of the workspace
func (w *Workspace) IsOwner(db *gorm.DB, userID int) (bool, error) {
	role, err := w.GetMemberRole(db, userID)
	if err != nil {
		return false, err
	}
	return role == RoleOwner, nil
}

// IsAdmin checks if a user is an admin of the workspace
func (w *Workspace) IsAdmin(db *gorm.DB, userID int) (bool, error) {
	role, err := w.GetMemberRole(db, userID)
	if err != nil {
		return false, err
	}
	return role == RoleAdmin || role == RoleOwner, nil
}

// validateOwnerRemoval ensures workspace has at least one owner
func (w *Workspace) validateOwnerRemoval(db *gorm.DB, userID int) error {
	var ownerCount int64
	err := db.Model(&WorkspaceMembership{}).
		Where("workspace_id = ? AND role = ? AND status = ? AND user_id != ?",
			w.ID, RoleOwner, StatusActive, userID).
		Count(&ownerCount).Error
	if err != nil {
		return err
	}
	if ownerCount == 0 {
		return errors.New("workspace must have at least one owner")
	}
	return nil
}

// GetOwners retrieves all owners of the workspace
func (w *Workspace) GetOwners(db *gorm.DB) ([]User, error) {
	var users []User
	err := db.Joins("JOIN workspace_memberships ON workspace_memberships.user_id = users.id").
		Where("workspace_memberships.workspace_id = ? AND workspace_memberships.role = ? AND workspace_memberships.status = ?",
			w.ID, RoleOwner, StatusActive).
		Find(&users).Error
	return users, err
}

// InviteMember creates an invitation for a user to join the workspace
func (w *Workspace) InviteMember(db *gorm.DB, inviterID int, inviteeEmail string, role MembershipRole) (*WorkspaceInvitation, error) {
	// Check if user already exists and is a member
	var user User
	if err := db.Where("email = ?", inviteeEmail).First(&user).Error; err == nil {
		// User exists, check membership
		isMember, _ := w.HasMember(db, user.ID)
		if isMember {
			return nil, errors.New("user is already a member of this workspace")
		}
	}

	// Check for existing pending invitation
	var existingInvite WorkspaceInvitation
	err := db.Where("workspace_id = ? AND invitee_email = ? AND status = ?",
		w.ID, inviteeEmail, "pending").First(&existingInvite).Error
	if err == nil {
		return nil, errors.New("invitation already sent to this email")
	}

	// Create invitation
	invitation := &WorkspaceInvitation{
		WorkspaceID:  w.ID,
		InviterID:    inviterID,
		InviteeEmail: inviteeEmail,
		Role:         string(role),
		Status:       "pending",
		ExpiresAt:    time.Now().Add(7 * 24 * time.Hour), // 7 days
	}

	if err := db.Create(invitation).Error; err != nil {
		return nil, err
	}

	return invitation, nil
}

// WorkspaceMember represents a member view with user details and role
type WorkspaceMember struct {
	ID        int       `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	JoinedAt  time.Time `json:"joined_at"`
}
