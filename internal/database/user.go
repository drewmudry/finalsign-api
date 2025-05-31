package database

import (
	"database/sql"
	"fmt"
	"time"
)

type User struct {
	ID         int       `json:"id"`
	Provider   string    `json:"provider"`
	ProviderID string    `json:"provider_id"`
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	AvatarURL  string    `json:"avatar_url"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreateOrUpdateUser creates a new user or updates an existing one
// If it's a new user, it also creates a personal workspace
func (s *service) CreateOrUpdateUser(user *User) error {
	// First, check if user already exists
	_, err := s.GetUserByProviderID(user.Provider, user.ProviderID)
	isNewUser := err != nil && err == sql.ErrNoRows

	// Create or update the user
	query := `
		INSERT INTO users (provider, provider_id, email, name, avatar_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (provider, provider_id) 
		DO UPDATE SET 
			email = EXCLUDED.email,
			name = EXCLUDED.name,
			avatar_url = EXCLUDED.avatar_url,
			updated_at = NOW()
		RETURNING id, created_at, updated_at`

	err = s.db.QueryRow(query, user.Provider, user.ProviderID, user.Email, user.Name, user.AvatarURL).
		Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return err
	}

	// If this is a new user, create their personal workspace
	if isNewUser {
		workspaceName := fmt.Sprintf("%s's Workspace", user.Name)
		_, err = s.CreateWorkspaceForUser(user.ID, workspaceName)
		if err != nil {
			// Log the error but don't fail the user creation
			// In production, you might want to handle this differently
			fmt.Printf("Warning: Failed to create workspace for new user %d: %v\n", user.ID, err)
		}
	}

	return nil
}

// GetUserByProviderID retrieves a user by provider and provider ID
func (s *service) GetUserByProviderID(provider, providerID string) (*User, error) {
	user := &User{}
	query := `SELECT id, provider, provider_id, email, name, avatar_url, created_at, updated_at 
			  FROM users WHERE provider = $1 AND provider_id = $2`

	err := s.db.QueryRow(query, provider, providerID).Scan(
		&user.ID, &user.Provider, &user.ProviderID, &user.Email,
		&user.Name, &user.AvatarURL, &user.CreatedAt, &user.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return user, nil
}

// GetUserByEmail retrieves a user by email
func (s *service) GetUserByEmail(email string) (*User, error) {
	user := &User{}
	query := `SELECT id, provider, provider_id, email, name, avatar_url, created_at, updated_at 
			  FROM users WHERE email = $1`

	err := s.db.QueryRow(query, email).Scan(
		&user.ID, &user.Provider, &user.ProviderID, &user.Email,
		&user.Name, &user.AvatarURL, &user.CreatedAt, &user.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return user, nil
}

// GetUserByID retrieves a user by ID
func (s *service) GetUserByID(id int) (*User, error) {
	user := &User{}
	query := `SELECT id, provider, provider_id, email, name, avatar_url, created_at, updated_at 
			  FROM users WHERE id = $1`

	err := s.db.QueryRow(query, id).Scan(
		&user.ID, &user.Provider, &user.ProviderID, &user.Email,
		&user.Name, &user.AvatarURL, &user.CreatedAt, &user.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return user, nil
}