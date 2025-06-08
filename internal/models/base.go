package models

import (
	"time"

	"gorm.io/gorm"
)

// Custom types to match PostgreSQL enums
type WorkspacePlan string
type MembershipRole string
type MembershipStatus string

const (
	// Workspace Plans
	PlanFree       WorkspacePlan = "free"
	PlanPro        WorkspacePlan = "pro"
	PlanEnterprise WorkspacePlan = "enterprise"

	// Membership Roles
	RoleOwner  MembershipRole = "owner"
	RoleAdmin  MembershipRole = "admin"
	RoleMember MembershipRole = "member"
	RoleViewer MembershipRole = "viewer"

	// Membership Status
	StatusActive    MembershipStatus = "active"
	StatusSuspended MembershipStatus = "suspended"
	StatusInvited   MembershipStatus = "invited"
)

// BaseModel contains common fields for all models
type BaseModel struct {
	CreatedAt time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index;column:deleted_at" json:"deleted_at,omitempty"`
}

// BeforeCreate hook for UUID generation
func (b *BaseModel) BeforeCreate(tx *gorm.DB) error {
	return nil
}
