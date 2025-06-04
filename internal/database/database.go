package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/joho/godotenv/autoload"
)

// Service represents a service that interacts with a database.
type Service interface {
	// Health returns a map of health status information.
	Health() map[string]string
	// Close terminates the database connection.
	Close() error

	// User operations
	CreateOrUpdateUser(user *User) error
	GetUserByProviderID(provider, providerID string) (*User, error)
	GetUserByEmail(email string) (*User, error)
	GetUserByID(id int) (*User, error)

	// Workspace operations
	CreateWorkspaceForUser(userID int, workspaceName string) (*Workspace, error)
	GetUserWorkspaces(userID int) ([]UserWorkspace, error)
	GetWorkspaceBySlug(slug string) (*Workspace, error)
	CheckUserWorkspaceAccess(userID int, workspaceSlug string) (*UserWorkspace, error)
	InviteUserToWorkspace(workspaceID uuid.UUID, invitedEmail string, inviterUserID int, role string) error
	AcceptWorkspaceInvitationByToken(token string, userID int) error
	DeclineWorkspaceInvitation(token string, userID int) error

	// NEW: Workspace management methods
	UpdateWorkspace(workspaceID uuid.UUID, name, description, settings string, userID int) error
	GetWorkspaceMembers(workspaceID uuid.UUID, userID int) ([]WorkspaceMember, error)
	GetWorkspacePendingInvitations(workspaceID uuid.UUID, userID int) ([]WorkspaceInvitation, error)
	UpdateMemberRole(workspaceID uuid.UUID, memberUserID int, newRole string, updaterUserID int) error
	RemoveMemberFromWorkspace(workspaceID uuid.UUID, memberUserID int, removerUserID int) error
	CancelWorkspaceInvitation(invitationID uuid.UUID, userID int) error

	// Invitation operations
	GetPendingInvitationByToken(token string) (*PendingInvitation, error)
	GetUserInvitations(userID int) ([]*PendingInvitation, error)

	// Notification operations
	CreateNotification(notification *Notification) error
	GetUserNotifications(userID int, limit int) ([]*Notification, error)
	MarkNotificationAsRead(notificationID uuid.UUID, userID int) error

	// Template operations
	CreateTemplate(template *Template, fields []TemplateField) (*Template, error)
	GetTemplateByID(templateID uuid.UUID, userID int) (*Template, error)
	GetTemplateWithFields(templateID uuid.UUID, userID int) (*TemplateWithFields, error)
	GetTemplateFields(templateID uuid.UUID, userID int) ([]TemplateField, error)
	GetWorkspaceTemplates(workspaceID uuid.UUID, userID int) ([]UserTemplate, error)
	UpdateTemplate(templateID uuid.UUID, name, description string, userID int) error
	DeactivateTemplate(templateID uuid.UUID, userID int) error
	AddFieldsToTemplate(templateID uuid.UUID, fields []TemplateField, userID int) error
}

type service struct {
	db *sql.DB
}

var (
	// db_string  = os.Getenv("DB_STRING") // Remove from here
	dbInstance *service
)

func New() Service {
	// Reuse Connection
	if dbInstance != nil {
		return dbInstance
	}
	db_string := os.Getenv("DB_STRING") // Add here
	connStr := db_string
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		log.Fatal(err)
	}
	dbInstance = &service{
		db: db,
	}
	return dbInstance
}

// Health checks the health of the database connection by pinging the database.
// It returns a map with keys indicating various health statistics.
func (s *service) Health() map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	stats := make(map[string]string)

	// Ping the database
	err := s.db.PingContext(ctx)
	if err != nil {
		stats["status"] = "down"
		stats["error"] = fmt.Sprintf("db down: %v", err)
		log.Fatalf("db down: %v", err) // Log the error and terminate the program
		return stats
	}

	// Database is up, add more statistics
	stats["status"] = "up"
	stats["message"] = "It's healthy"

	// Get database stats (like open connections, in use, idle, etc.)
	dbStats := s.db.Stats()
	stats["open_connections"] = strconv.Itoa(dbStats.OpenConnections)
	stats["in_use"] = strconv.Itoa(dbStats.InUse)
	stats["idle"] = strconv.Itoa(dbStats.Idle)
	stats["wait_count"] = strconv.FormatInt(dbStats.WaitCount, 10)
	stats["wait_duration"] = dbStats.WaitDuration.String()
	stats["max_idle_closed"] = strconv.FormatInt(dbStats.MaxIdleClosed, 10)
	stats["max_lifetime_closed"] = strconv.FormatInt(dbStats.MaxLifetimeClosed, 10)

	// Evaluate stats to provide a health message
	if dbStats.OpenConnections > 40 { // Assuming 50 is the max for this example
		stats["message"] = "The database is experiencing heavy load."
	}

	if dbStats.WaitCount > 1000 {
		stats["message"] = "The database has a high number of wait events, indicating potential bottlenecks."
	}

	if dbStats.MaxIdleClosed > int64(dbStats.OpenConnections)/2 {
		stats["message"] = "Many idle connections are being closed, consider revising the connection pool settings."
	}

	if dbStats.MaxLifetimeClosed > int64(dbStats.OpenConnections)/2 {
		stats["message"] = "Many connections are being closed due to max lifetime, consider increasing max lifetime or revising the connection usage pattern."
	}

	return stats
}

// Close closes the database connection.
// It logs a message indicating the disconnection from the specific database.
// If the connection is successfully closed, it returns nil.
// If an error occurs while closing the connection, it returns the error.
func (s *service) Close() error {
	// log.Printf("Disconnected from database: %s", database)
	return s.db.Close()
}
