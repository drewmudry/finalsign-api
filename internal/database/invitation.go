// Add to internal/database/invitation.go
package database

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Invitation struct {
	ID            uuid.UUID  `json:"id"`
	WorkspaceID   uuid.UUID  `json:"workspace_id"`
	InviterID     int        `json:"inviter_id"`
	InviteeEmail  string     `json:"invitee_email"`
	InviteeID     *int       `json:"invitee_id,omitempty"`
	Role          string     `json:"role"`
	Status        string     `json:"status"`
	Token         string     `json:"token"`
	ExpiresAt     time.Time  `json:"expires_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	AcceptedAt    *time.Time `json:"accepted_at,omitempty"`
	DeclinedAt    *time.Time `json:"declined_at,omitempty"`
}

type PendingInvitation struct {
	ID            uuid.UUID  `json:"id"`
	WorkspaceID   uuid.UUID  `json:"workspace_id"`
	WorkspaceName string     `json:"workspace_name"`
	WorkspaceSlug string     `json:"workspace_slug"`
	InviterID     int        `json:"inviter_id"`
	InviterName   string     `json:"inviter_name"`
	InviterEmail  string     `json:"inviter_email"`
	InviteeEmail  string     `json:"invitee_email"`
	InviteeID     *int       `json:"invitee_id,omitempty"`
	InviteeName   *string    `json:"invitee_name,omitempty"`
	Role          string     `json:"role"`
	Token         string     `json:"token"`
	ExpiresAt     time.Time  `json:"expires_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

type Notification struct {
	ID        uuid.UUID  `json:"id"`
	UserID    int        `json:"user_id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Message   string     `json:"message"`
	Data      string     `json:"data"` // JSON string
	IsRead    bool       `json:"is_read"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}



// GetPendingInvitationByToken retrieves a pending invitation by token
func (s *service) GetPendingInvitationByToken(token string) (*PendingInvitation, error) {
	invitation := &PendingInvitation{}
	query := `
		SELECT id, workspace_id, workspace_name, workspace_slug, inviter_id, inviter_name, 
			   inviter_email, invitee_email, invitee_id, invitee_name, role, token, expires_at, created_at
		FROM pending_invitations
		WHERE token = $1
	`

	err := s.db.QueryRow(query, token).Scan(
		&invitation.ID,
		&invitation.WorkspaceID,
		&invitation.WorkspaceName,
		&invitation.WorkspaceSlug,
		&invitation.InviterID,
		&invitation.InviterName,
		&invitation.InviterEmail,
		&invitation.InviteeEmail,
		&invitation.InviteeID,
		&invitation.InviteeName,
		&invitation.Role,
		&invitation.Token,
		&invitation.ExpiresAt,
		&invitation.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("invitation not found or expired: %w", err)
	}

	return invitation, nil
}


// GetUserInvitations retrieves all invitations for a user
func (s *service) GetUserInvitations(userID int) ([]*PendingInvitation, error) {
	query := `
		SELECT id, workspace_id, workspace_name, workspace_slug, inviter_id, inviter_name, 
			   inviter_email, invitee_email, invitee_id, invitee_name, role, token, expires_at, created_at
		FROM pending_invitations
		WHERE invitee_id = $1
		ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user invitations: %w", err)
	}
	defer rows.Close()

	var invitations []*PendingInvitation
	for rows.Next() {
		invitation := &PendingInvitation{}
		err := rows.Scan(
			&invitation.ID,
			&invitation.WorkspaceID,
			&invitation.WorkspaceName,
			&invitation.WorkspaceSlug,
			&invitation.InviterID,
			&invitation.InviterName,
			&invitation.InviterEmail,
			&invitation.InviteeEmail,
			&invitation.InviteeID,
			&invitation.InviteeName,
			&invitation.Role,
			&invitation.Token,
			&invitation.ExpiresAt,
			&invitation.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan invitation: %w", err)
		}
		invitations = append(invitations, invitation)
	}

	return invitations, nil
}

// CreateNotification creates a new notification
func (s *service) CreateNotification(notification *Notification) error {
	query := `
		INSERT INTO notifications (user_id, type, title, message, data, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`

	err := s.db.QueryRow(
		query,
		notification.UserID,
		notification.Type,
		notification.Title,
		notification.Message,
		notification.Data,
		notification.ExpiresAt,
	).Scan(&notification.ID, &notification.CreatedAt)

	return err
}

// GetUserNotifications retrieves notifications for a user
func (s *service) GetUserNotifications(userID int, limit int) ([]*Notification, error) {
	query := `
		SELECT id, user_id, type, title, message, data, is_read, created_at, expires_at
		FROM user_notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := s.db.Query(query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get notifications: %w", err)
	}
	defer rows.Close()

	var notifications []*Notification
	for rows.Next() {
		notification := &Notification{}
		err := rows.Scan(
			&notification.ID,
			&notification.UserID,
			&notification.Type,
			&notification.Title,
			&notification.Message,
			&notification.Data,
			&notification.IsRead,
			&notification.CreatedAt,
			&notification.ExpiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification: %w", err)
		}
		notifications = append(notifications, notification)
	}

	return notifications, nil
}

// MarkNotificationAsRead marks a notification as read
func (s *service) MarkNotificationAsRead(notificationID uuid.UUID, userID int) error {
	result, err := s.db.Exec(`
		UPDATE notifications 
		SET is_read = TRUE 
		WHERE id = $1 AND user_id = $2
	`, notificationID, userID)

	if err != nil {
		return fmt.Errorf("failed to mark notification as read: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("notification not found")
	}

	return nil
}