package models

import (
	"fmt"
	"log"
)

// ExampleUsage demonstrates how to use the Django-like ORM interface
func ExampleUsage() {
	// Initialize database connection
	db, err := NewDB()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Example 1: Create a user using manager (Django style)
	user, created, err := db.Users.GetOrCreate("google", "123456", User{
		Email:     "john@example.com",
		Name:      "John Doe",
		AvatarURL: "https://example.com/avatar.jpg",
	})
	if err != nil {
		log.Fatal("Failed to create user:", err)
	}
	if created {
		fmt.Println("Created new user")
	}

	// Example 2: Create a workspace
	workspace := &Workspace{
		Name:        "My Awesome Workspace",
		Description: "A workspace for awesome projects",
		Plan:        PlanFree,
		IsActive:    true,
	}
	if err := db.Workspaces.Create(workspace); err != nil {
		log.Fatal("Failed to create workspace:", err)
	}

	// Example 3: Add member to workspace (Django style!)
	// This is what you asked for: workspace.AddMember(user)
	if err := workspace.AddMember(db.DB, user, RoleOwner); err != nil {
		log.Fatal("Failed to add member:", err)
	}

	// Example 4: Using transactions
	err = db.Transaction(func(txDB *DB) error {
		// Create another user
		newUser := &User{
			Provider:   "github",
			ProviderID: "789",
			Email:      "jane@example.com",
			Name:       "Jane Smith",
		}
		if err := txDB.Users.Create(newUser); err != nil {
			return err
		}

		// Add as member
		if err := workspace.AddMember(txDB.DB, newUser, RoleMember); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		log.Fatal("Transaction failed:", err)
	}

	// Example 5: Query workspace members
	members, err := workspace.GetMembers(db.DB)
	if err != nil {
		log.Fatal("Failed to get members:", err)
	}
	fmt.Printf("Workspace has %d members\n", len(members))

	// Example 6: Check user permissions
	isOwner, err := workspace.IsOwner(db.DB, user.ID)
	if err != nil {
		log.Fatal("Failed to check ownership:", err)
	}
	fmt.Printf("User is owner: %v\n", isOwner)

	// Example 7: Get user's workspaces
	userWorkspaces, err := user.GetActiveWorkspaces(db.DB)
	if err != nil {
		log.Fatal("Failed to get user workspaces:", err)
	}
	fmt.Printf("User belongs to %d workspaces\n", len(userWorkspaces))

	// Example 8: Update member role
	jane, _ := db.Users.GetByEmail("jane@example.com")
	if err := workspace.UpdateMemberRole(db.DB, jane, RoleAdmin); err != nil {
		log.Fatal("Failed to update role:", err)
	}

	// Example 9: Invite a new member
	invitation, err := workspace.InviteMember(db.DB, user.ID, "newuser@example.com", RoleMember)
	if err != nil {
		log.Fatal("Failed to create invitation:", err)
	}
	fmt.Printf("Invitation created with token: %s\n", invitation.Token)

	// Example 10: Accept invitation
	newUser, _ := db.Users.GetByEmail("newuser@example.com")
	if newUser != nil {
		if err := invitation.Accept(db.DB, newUser.ID); err != nil {
			log.Fatal("Failed to accept invitation:", err)
		}
	}

	// Example 11: Using Django-like helper functions
	workspaceExists, _ := Exists[Workspace](db.DB, "slug = ?", workspace.Slug)
	fmt.Printf("Workspace exists: %v\n", workspaceExists)

	memberCount, _ := Count[WorkspaceMembership](db.DB, "workspace_id = ?", workspace.ID)
	fmt.Printf("Workspace has %d members\n", memberCount)

	// Example 12: Remove member
	if err := workspace.RemoveMember(db.DB, jane); err != nil {
		log.Fatal("Failed to remove member:", err)
	}
}

// More Django-like patterns you can use:

// CreateWorkspaceWithOwner creates a workspace and adds the owner in one transaction
func CreateWorkspaceWithOwner(db *DB, workspaceName string, owner *User) (*Workspace, error) {
	var workspace *Workspace

	err := db.Transaction(func(txDB *DB) error {
		// Create workspace
		workspace = &Workspace{
			Name:        workspaceName,
			Description: fmt.Sprintf("%s's workspace", owner.Name),
			Plan:        PlanFree,
			IsActive:    true,
		}
		if err := txDB.Workspaces.Create(workspace); err != nil {
			return err
		}

		// Add owner
		return workspace.AddMember(txDB.DB, owner, RoleOwner)
	})

	return workspace, err
}

// GetOrCreateWorkspace gets or creates a workspace by slug
func GetOrCreateWorkspace(db *DB, slug string, defaults Workspace) (*Workspace, bool, error) {
	workspace, err := db.Workspaces.GetBySlug(slug)
	if err == nil {
		return workspace, false, nil
	}

	// Create new workspace
	defaults.Slug = slug
	if err := db.Workspaces.Create(&defaults); err != nil {
		return nil, false, err
	}

	return &defaults, true, nil
}
