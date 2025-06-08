package models

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MigrateAdapter provides migration functionality compatible with existing system
type MigrateAdapter struct {
	db *gorm.DB
}

// NewMigrateAdapter creates a new migration adapter
func NewMigrateAdapter(db *gorm.DB) *MigrateAdapter {
	return &MigrateAdapter{db: db}
}

// RunMigrations runs the existing SQL migrations
func (m *MigrateAdapter) RunMigrations() error {
	// Get the underlying SQL database from GORM
	sqlDB, err := m.db.DB()
	if err != nil {
		return fmt.Errorf("could not get sql.DB from gorm: %w", err)
	}

	// Create migration driver
	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("could not create migration driver: %w", err)
	}

	// Create migration instance
	migration, err := migrate.NewWithDatabaseInstance(
		"file://migrations",
		"postgres", driver)
	if err != nil {
		return fmt.Errorf("could not create migration instance: %w", err)
	}

	// Run migrations
	if err := migration.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("could not run migrations: %w", err)
	}

	return nil
}

// GetMigrationVersion gets the current migration version
func (m *MigrateAdapter) GetMigrationVersion() (uint, bool, error) {
	sqlDB, err := m.db.DB()
	if err != nil {
		return 0, false, err
	}

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return 0, false, err
	}

	migration, err := migrate.NewWithDatabaseInstance(
		"file://migrations",
		"postgres", driver)
	if err != nil {
		return 0, false, err
	}

	version, dirty, err := migration.Version()
	return version, dirty, err
}

// CreateDatabaseService creates a backward-compatible database service
// This allows gradual migration from the old system to GORM
type DatabaseService struct {
	*DB
	sqlDB *sql.DB
}

// NewDatabaseService creates a new database service that's compatible with the old interface
func NewDatabaseService() (*DatabaseService, error) {
	// Create GORM connection
	db, err := NewDB()
	if err != nil {
		return nil, err
	}

	// Get SQL database for backward compatibility
	sqlDB, err := db.DB.DB()
	if err != nil {
		return nil, err
	}

	return &DatabaseService{
		DB:    db,
		sqlDB: sqlDB,
	}, nil
}

// GetSQLDB returns the underlying sql.DB for backward compatibility
func (s *DatabaseService) GetSQLDB() *sql.DB {
	return s.sqlDB
}

// Example of how to gradually migrate existing methods to GORM
func (s *DatabaseService) CreateOrUpdateUser(user *User) error {
	// Using GORM instead of raw SQL
	result := s.DB.DB.Where("provider = ? AND provider_id = ?", user.Provider, user.ProviderID).
		Assign(User{
			Email:     user.Email,
			Name:      user.Name,
			AvatarURL: user.AvatarURL,
		}).
		FirstOrCreate(user)

	if result.Error != nil {
		return result.Error
	}

	// If new user was created, create personal workspace
	if result.RowsAffected > 0 {
		workspace, err := user.CreatePersonalWorkspace(s.DB.DB)
		if err != nil {
			// Log but don't fail
			fmt.Printf("Warning: Failed to create workspace for new user %d: %v\n", user.ID, err)
		} else {
			fmt.Printf("Created personal workspace %s for user %d\n", workspace.Slug, user.ID)
		}
	}

	return nil
}

// GetUserByProviderID using GORM
func (s *DatabaseService) GetUserByProviderID(provider, providerID string) (*User, error) {
	return s.Users.GetByProvider(provider, providerID)
}

// GetUserByEmail using GORM
func (s *DatabaseService) GetUserByEmail(email string) (*User, error) {
	return s.Users.GetByEmail(email)
}

// GetUserByID using GORM
func (s *DatabaseService) GetUserByID(id int) (*User, error) {
	return s.Users.Get(id)
}

// CreateWorkspaceForUser using GORM with transaction
func (s *DatabaseService) CreateWorkspaceForUser(userID int, workspaceName string) (*Workspace, error) {
	user, err := s.Users.Get(userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return CreateWorkspaceWithOwner(s.DB, workspaceName, user)
}

// GetUserWorkspaces using GORM
func (s *DatabaseService) GetUserWorkspaces(userID int) ([]UserWorkspace, error) {
	var userWorkspaces []UserWorkspace

	query := `
		SELECT 
			u.id as user_id,
			u.email,
			u.name as user_name,
			w.id as workspace_id,
			w.name as workspace_name,
			w.slug as workspace_slug,
			wm.role,
			wm.status as membership_status,
			wm.joined_at,
			w.plan,
			w.is_active as workspace_active
		FROM users u
		JOIN workspace_memberships wm ON u.id = wm.user_id
		JOIN workspaces w ON wm.workspace_id = w.id
		WHERE u.id = ? 
		AND wm.status = 'active' 
		AND w.is_active = true
		ORDER BY wm.joined_at ASC`

	err := s.DB.DB.Raw(query, userID).Scan(&userWorkspaces).Error
	return userWorkspaces, err
}

// UserWorkspace represents the view model for backward compatibility
type UserWorkspace struct {
	UserID           int       `json:"user_id"`
	Email            string    `json:"email"`
	UserName         string    `json:"user_name"`
	WorkspaceID      uuid.UUID `json:"workspace_id"`
	WorkspaceName    string    `json:"workspace_name"`
	WorkspaceSlug    string    `json:"workspace_slug"`
	Role             string    `json:"role"`
	MembershipStatus string    `json:"membership_status"`
	JoinedAt         time.Time `json:"joined_at"`
	Plan             string    `json:"plan"`
	WorkspaceActive  bool      `json:"workspace_active"`
}
