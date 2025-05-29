package database

import (
	"time"
)

type Waitlist struct {
	ID         int        `json:"id"`
	Email      string     `json:"email"`
	Name       string     `json:"name"`
	Phone      string     `json:"phone"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	NotifiedAt *time.Time `json:"notified_at,omitempty"`
}

// CreateOrUpdateWaitlistEntry creates a new waitlist entry or updates existing one
func (s *service) CreateWaitlistEntry(waitlist *Waitlist) error {
	query := `
		INSERT INTO waitlist (email, name, phone, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (email) DO UPDATE SET
			name = EXCLUDED.name,
			phone = EXCLUDED.phone,
			updated_at = NOW()
		RETURNING id, created_at, updated_at`

	err := s.db.QueryRow(query, waitlist.Email, waitlist.Name, waitlist.Phone).
		Scan(&waitlist.ID, &waitlist.CreatedAt, &waitlist.UpdatedAt)

	return err
}
