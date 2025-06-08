package models

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB holds the database connection and all model managers
type DB struct {
	*gorm.DB
	Users                *UserManager
	Workspaces           *WorkspaceManager
	Memberships          *WorkspaceMembershipManager
	WorkspaceInvitations *WorkspaceInvitationManager
}

// NewDB creates a new database connection and initializes all managers
func NewDB() (*DB, error) {
	dsn := os.Getenv("DB_STRING")
	if dsn == "" {
		return nil, fmt.Errorf("DB_STRING environment variable not set")
	}

	// Configure GORM
	config := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	}

	// Open database connection
	gormDB, err := gorm.Open(postgres.Open(dsn), config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Create DB instance with managers
	db := &DB{
		DB:                   gormDB,
		Users:                NewUserManager(gormDB),
		Workspaces:           NewWorkspaceManager(gormDB),
		Memberships:          NewWorkspaceMembershipManager(gormDB),
		WorkspaceInvitations: NewWorkspaceInvitationManager(gormDB),
	}

	// Auto-migrate models (optional, can be disabled in production)
	if err := db.AutoMigrate(); err != nil {
		log.Printf("Warning: AutoMigrate failed: %v", err)
	}

	return db, nil
}

// AutoMigrate runs GORM auto-migration for all models
func (db *DB) AutoMigrate() error {
	return db.DB.AutoMigrate(
		&User{},
		&Workspace{},
		&WorkspaceMembership{},
		&WorkspaceInvitation{},
	)
}

// Transaction runs a function within a database transaction
func (db *DB) Transaction(fn func(*DB) error) error {
	return db.DB.Transaction(func(tx *gorm.DB) error {
		// Create a new DB instance with the transaction
		txDB := &DB{
			DB:                   tx,
			Users:                NewUserManager(tx),
			Workspaces:           NewWorkspaceManager(tx),
			Memberships:          NewWorkspaceMembershipManager(tx),
			WorkspaceInvitations: NewWorkspaceInvitationManager(tx),
		}
		return fn(txDB)
	})
}

// Close closes the database connection
func (db *DB) Close() error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Django-like convenience methods

// GetObjectOr404 retrieves an object or returns an error (similar to Django's get_object_or_404)
func GetObjectOr404[T any](db *gorm.DB, conditions ...interface{}) (*T, error) {
	var obj T
	err := db.First(&obj, conditions...).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("object not found")
		}
		return nil, err
	}
	return &obj, nil
}

// Exists checks if a record exists (similar to Django's exists())
func Exists[T any](db *gorm.DB, conditions ...interface{}) (bool, error) {
	var count int64
	err := db.Model(new(T)).Where(conditions[0], conditions[1:]...).Count(&count).Error
	return count > 0, err
}

// BulkCreate creates multiple records (similar to Django's bulk_create)
func BulkCreate[T any](db *gorm.DB, objects []T) error {
	if len(objects) == 0 {
		return nil
	}
	return db.CreateInBatches(objects, 100).Error
}

// Count returns the count of records (similar to Django's count())
func Count[T any](db *gorm.DB, conditions ...interface{}) (int64, error) {
	var count int64
	query := db.Model(new(T))
	if len(conditions) > 0 {
		query = query.Where(conditions[0], conditions[1:]...)
	}
	err := query.Count(&count).Error
	return count, err
}
