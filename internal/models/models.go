// Package models provides GORM-based models with a Django ORM-like interface
// for managing users, workspaces, and memberships.
package models

// This file re-exports all types for convenient importing
// Usage: import "yourproject/internal/models"

import (
	"time"

	"github.com/google/uuid"
)

// Re-export all types from other files
// This allows users to do: models.User instead of models.user.User

// PendingInvitation represents a simplified view of WorkspaceInvitation
// for backward compatibility with existing code
type PendingInvitation struct {
	ID            uuid.UUID `json:"id"`
	WorkspaceID   uuid.UUID `json:"workspace_id"`
	WorkspaceName string    `json:"workspace_name"`
	InviterID     int       `json:"inviter_id"`
	InviterName   string    `json:"inviter_name"`
	InviteeEmail  string    `json:"invitee_email"`
	InviteeID     *int      `json:"invitee_id"`
	Role          string    `json:"role"`
	Status        string    `json:"status"`
	Token         string    `json:"token"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// Notification represents a user notification (placeholder for future implementation)
type Notification struct {
	ID        uuid.UUID `json:"id"`
	UserID    int       `json:"user_id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Data      string    `json:"data"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

// Template-related types (placeholder for future implementation)
type Template struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TemplateSigner struct {
	ID         uuid.UUID `json:"id"`
	TemplateID uuid.UUID `json:"template_id"`
	Name       string    `json:"name"`
	Email      string    `json:"email"`
	Order      int       `json:"order"`
}

type TemplateField struct {
	ID         uuid.UUID `json:"id"`
	TemplateID uuid.UUID `json:"template_id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Required   bool      `json:"required"`
	Order      int       `json:"order"`
}

type TemplateWithSignersAndFields struct {
	Template
	Signers []TemplateSigner `json:"signers"`
	Fields  []TemplateField  `json:"fields"`
}

type TemplateListItem struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}
