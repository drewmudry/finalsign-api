package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	ID         int       `gorm:"primaryKey;column:id" json:"id"`
	Provider   string    `gorm:"column:provider;not null" json:"provider"`
	ProviderID string    `gorm:"column:provider_id;not null" json:"provider_id"`
	Email      string    `gorm:"column:email;uniqueIndex;not null" json:"email"`
	Name       string    `gorm:"column:name;not null" json:"name"`
	AvatarURL  string    `gorm:"column:avatar_url" json:"avatar_url"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at" json:"updated_at"`

	// Associations
	Memberships []WorkspaceMembership `gorm:"foreignKey:UserID" json:"memberships,omitempty"`
	Workspaces  []Workspace           `gorm:"many2many:workspace_memberships;" json:"workspaces,omitempty"`
}

// TableName specifies the table name for the User model
func (User) TableName() string {
	return "users"
}

// UserManager provides Django-like ORM methods for User
type UserManager struct {
	db *gorm.DB
}

// NewUserManager creates a new UserManager instance
func NewUserManager(db *gorm.DB) *UserManager {
	return &UserManager{db: db}
}

// Create creates a new user
func (m *UserManager) Create(user *User) error {
	return m.db.Create(user).Error
}

// GetOrCreate gets an existing user or creates a new one
func (m *UserManager) GetOrCreate(provider, providerID string, defaults User) (*User, bool, error) {
	var user User
	created := false

	err := m.db.Where("provider = ? AND provider_id = ?", provider, providerID).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		// Create new user
		user = defaults
		user.Provider = provider
		user.ProviderID = providerID
		if err := m.db.Create(&user).Error; err != nil {
			return nil, false, err
		}
		created = true
	} else if err != nil {
		return nil, false, err
	}

	return &user, created, nil
}

// Get retrieves a user by ID
func (m *UserManager) Get(id int) (*User, error) {
	var user User
	err := m.db.First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByEmail retrieves a user by email
func (m *UserManager) GetByEmail(email string) (*User, error) {
	var user User
	err := m.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByProvider retrieves a user by provider and provider ID
func (m *UserManager) GetByProvider(provider, providerID string) (*User, error) {
	var user User
	err := m.db.Where("provider = ? AND provider_id = ?", provider, providerID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// All retrieves all users
func (m *UserManager) All() ([]User, error) {
	var users []User
	err := m.db.Find(&users).Error
	return users, err
}

// Filter retrieves users matching the given conditions
func (m *UserManager) Filter(conditions interface{}) ([]User, error) {
	var users []User
	err := m.db.Where(conditions).Find(&users).Error
	return users, err
}

// Update updates a user
func (m *UserManager) Update(user *User) error {
	return m.db.Save(user).Error
}

// Delete soft deletes a user
func (m *UserManager) Delete(id int) error {
	return m.db.Delete(&User{}, id).Error
}

// Django-like instance methods for User

// Save saves the user instance
func (u *User) Save(db *gorm.DB) error {
	return db.Save(u).Error
}

// Delete soft deletes the user instance
func (u *User) Delete(db *gorm.DB) error {
	return db.Delete(u).Error
}

// GetWorkspaces retrieves all workspaces for the user
func (u *User) GetWorkspaces(db *gorm.DB) ([]Workspace, error) {
	var workspaces []Workspace
	err := db.Model(u).Association("Workspaces").Find(&workspaces)
	return workspaces, err
}

// GetActiveWorkspaces retrieves all active workspaces for the user
func (u *User) GetActiveWorkspaces(db *gorm.DB) ([]Workspace, error) {
	var workspaces []Workspace
	err := db.Joins("JOIN workspace_memberships ON workspace_memberships.workspace_id = workspaces.id").
		Where("workspace_memberships.user_id = ? AND workspace_memberships.status = ? AND workspaces.is_active = ?",
			u.ID, StatusActive, true).
		Find(&workspaces).Error
	return workspaces, err
}

// GetMemberships retrieves all workspace memberships for the user
func (u *User) GetMemberships(db *gorm.DB) ([]WorkspaceMembership, error) {
	var memberships []WorkspaceMembership
	err := db.Where("user_id = ?", u.ID).Find(&memberships).Error
	return memberships, err
}

// CreatePersonalWorkspace creates a personal workspace for the user
func (u *User) CreatePersonalWorkspace(db *gorm.DB) (*Workspace, error) {
	workspaceName := fmt.Sprintf("%s's Workspace", u.Name)
	workspace := &Workspace{
		Name:        workspaceName,
		Description: "Personal workspace",
		Plan:        PlanFree,
		IsActive:    true,
	}

	// Use transaction to ensure atomicity
	err := db.Transaction(func(tx *gorm.DB) error {
		// Create workspace
		if err := tx.Create(workspace).Error; err != nil {
			return err
		}

		// Create membership as owner
		membership := &WorkspaceMembership{
			WorkspaceID: workspace.ID,
			UserID:      u.ID,
			Role:        RoleOwner,
			Status:      StatusActive,
			JoinedAt:    time.Now(),
		}

		return tx.Create(membership).Error
	})

	if err != nil {
		return nil, err
	}

	return workspace, nil
}

// IsWorkspaceMember checks if user is a member of a workspace
func (u *User) IsWorkspaceMember(db *gorm.DB, workspaceID uuid.UUID) (bool, error) {
	var count int64
	err := db.Model(&WorkspaceMembership{}).
		Where("user_id = ? AND workspace_id = ? AND status = ?", u.ID, workspaceID, StatusActive).
		Count(&count).Error
	return count > 0, err
}

// GetWorkspaceRole gets the user's role in a workspace
func (u *User) GetWorkspaceRole(db *gorm.DB, workspaceID uuid.UUID) (MembershipRole, error) {
	var membership WorkspaceMembership
	err := db.Where("user_id = ? AND workspace_id = ? AND status = ?", u.ID, workspaceID, StatusActive).
		First(&membership).Error
	if err != nil {
		return "", err
	}
	return membership.Role, nil
}
