package models

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WorkspaceInvitation represents an invitation to join a workspace
type WorkspaceInvitation struct {
	ID           uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	WorkspaceID  uuid.UUID `gorm:"type:uuid;not null" json:"workspace_id"`
	InviterID    int       `gorm:"not null" json:"inviter_id"`
	InviteeEmail string    `gorm:"not null" json:"invitee_email"`
	InviteeID    *int      `gorm:"column:invitee_id" json:"invitee_id,omitempty"`
	Role         string    `gorm:"not null" json:"role"`
	Status       string    `gorm:"default:'pending'" json:"status"` // pending, accepted, declined, expired
	Token        string    `gorm:"uniqueIndex;not null" json:"token"`
	ExpiresAt    time.Time `gorm:"not null" json:"expires_at"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at" json:"updated_at"`

	// Associations
	Workspace Workspace `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
	Inviter   User      `gorm:"foreignKey:InviterID" json:"inviter,omitempty"`
	Invitee   *User     `gorm:"foreignKey:InviteeID" json:"invitee,omitempty"`
}

// TableName specifies the table name for the WorkspaceInvitation model
func (WorkspaceInvitation) TableName() string {
	return "workspace_invitations"
}

// BeforeCreate generates a unique token and sets expiry
func (wi *WorkspaceInvitation) BeforeCreate(tx *gorm.DB) error {
	// Generate a secure random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return err
	}
	wi.Token = hex.EncodeToString(tokenBytes)

	// Set expiry if not set (7 days default)
	if wi.ExpiresAt.IsZero() {
		wi.ExpiresAt = time.Now().Add(7 * 24 * time.Hour)
	}

	return nil
}

// WorkspaceInvitationManager provides Django-like ORM methods for WorkspaceInvitation
type WorkspaceInvitationManager struct {
	db *gorm.DB
}

// NewWorkspaceInvitationManager creates a new WorkspaceInvitationManager instance
func NewWorkspaceInvitationManager(db *gorm.DB) *WorkspaceInvitationManager {
	return &WorkspaceInvitationManager{db: db}
}

// Create creates a new workspace invitation
func (m *WorkspaceInvitationManager) Create(invitation *WorkspaceInvitation) error {
	return m.db.Create(invitation).Error
}

// Get retrieves an invitation by ID
func (m *WorkspaceInvitationManager) Get(id uuid.UUID) (*WorkspaceInvitation, error) {
	var invitation WorkspaceInvitation
	err := m.db.First(&invitation, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &invitation, nil
}

// GetByToken retrieves an invitation by token
func (m *WorkspaceInvitationManager) GetByToken(token string) (*WorkspaceInvitation, error) {
	var invitation WorkspaceInvitation
	err := m.db.Where("token = ?", token).First(&invitation).Error
	if err != nil {
		return nil, err
	}
	return &invitation, nil
}

// GetPendingByToken retrieves a pending invitation by token
func (m *WorkspaceInvitationManager) GetPendingByToken(token string) (*WorkspaceInvitation, error) {
	var invitation WorkspaceInvitation
	err := m.db.Where("token = ? AND status = ? AND expires_at > ?", token, "pending", time.Now()).
		Preload("Workspace").
		Preload("Inviter").
		First(&invitation).Error
	if err != nil {
		return nil, err
	}
	return &invitation, nil
}

// Filter retrieves invitations matching the given conditions
func (m *WorkspaceInvitationManager) Filter(conditions interface{}) ([]WorkspaceInvitation, error) {
	var invitations []WorkspaceInvitation
	err := m.db.Where(conditions).Find(&invitations).Error
	return invitations, err
}

// GetPendingForEmail retrieves all pending invitations for an email
func (m *WorkspaceInvitationManager) GetPendingForEmail(email string) ([]WorkspaceInvitation, error) {
	var invitations []WorkspaceInvitation
	err := m.db.Where("invitee_email = ? AND status = ? AND expires_at > ?", email, "pending", time.Now()).
		Preload("Workspace").
		Preload("Inviter").
		Find(&invitations).Error
	return invitations, err
}

// GetPendingForWorkspace retrieves all pending invitations for a workspace
func (m *WorkspaceInvitationManager) GetPendingForWorkspace(workspaceID uuid.UUID) ([]WorkspaceInvitation, error) {
	var invitations []WorkspaceInvitation
	err := m.db.Where("workspace_id = ? AND status = ? AND expires_at > ?", workspaceID, "pending", time.Now()).
		Preload("Inviter").
		Find(&invitations).Error
	return invitations, err
}

// Update updates an invitation
func (m *WorkspaceInvitationManager) Update(invitation *WorkspaceInvitation) error {
	return m.db.Save(invitation).Error
}

// Delete deletes an invitation
func (m *WorkspaceInvitationManager) Delete(id uuid.UUID) error {
	return m.db.Delete(&WorkspaceInvitation{}, "id = ?", id).Error
}

// Django-like instance methods for WorkspaceInvitation

// Save saves the invitation instance
func (wi *WorkspaceInvitation) Save(db *gorm.DB) error {
	return db.Save(wi).Error
}

// Delete deletes the invitation instance
func (wi *WorkspaceInvitation) Delete(db *gorm.DB) error {
	return db.Delete(wi).Error
}

// Accept accepts the invitation and creates a membership
func (wi *WorkspaceInvitation) Accept(db *gorm.DB, userID int) error {
	// Check if invitation is valid
	if !wi.IsValid() {
		return gorm.ErrRecordNotFound
	}

	// Use transaction
	return db.Transaction(func(tx *gorm.DB) error {
		// Update invitation status
		wi.Status = "accepted"
		wi.InviteeID = &userID
		if err := tx.Save(wi).Error; err != nil {
			return err
		}

		// Create membership
		membership := &WorkspaceMembership{
			WorkspaceID: wi.WorkspaceID,
			UserID:      userID,
			Role:        MembershipRole(wi.Role),
			Status:      StatusActive,
			InvitedBy:   &wi.InviterID,
			InvitedAt:   &wi.CreatedAt,
			JoinedAt:    time.Now(),
		}

		return tx.Create(membership).Error
	})
}

// Decline declines the invitation
func (wi *WorkspaceInvitation) Decline(db *gorm.DB) error {
	wi.Status = "declined"
	return db.Save(wi).Error
}

// Cancel cancels the invitation (by inviter)
func (wi *WorkspaceInvitation) Cancel(db *gorm.DB) error {
	wi.Status = "cancelled"
	return db.Save(wi).Error
}

// IsValid checks if the invitation is still valid
func (wi *WorkspaceInvitation) IsValid() bool {
	return wi.Status == "pending" && time.Now().Before(wi.ExpiresAt)
}

// IsExpired checks if the invitation has expired
func (wi *WorkspaceInvitation) IsExpired() bool {
	return time.Now().After(wi.ExpiresAt)
}

// GetInvitationLink generates the invitation link
func (wi *WorkspaceInvitation) GetInvitationLink(baseURL string) string {
	return baseURL + "/invitations/accept?token=" + wi.Token
}
