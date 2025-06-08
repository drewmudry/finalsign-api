# GORM Models with Django-like ORM

This package provides GORM-based models with a Django ORM-like interface for managing users, workspaces, and memberships.

## Features

- **Django-style model managers** for easy querying
- **Instance methods** on models for intuitive operations
- **Transaction support** for complex operations
- **Type-safe enums** for roles and statuses
- **Backward compatibility** with existing migrations

## Quick Start

```go
// Initialize the database
db, err := models.NewDB()
if err != nil {
    log.Fatal(err)
}
defer db.Close()

// Create a user
user := &models.User{
    Provider:   "google",
    ProviderID: "123456",
    Email:      "user@example.com",
    Name:       "John Doe",
}
err = db.Users.Create(user)

// Create a workspace
workspace := &models.Workspace{
    Name:        "My Workspace",
    Description: "A great workspace",
    Plan:        models.PlanFree,
}
err = db.Workspaces.Create(workspace)

// Add member to workspace (Django style!)
err = workspace.AddMember(db.DB, user, models.RoleOwner)
```

## Django-like ORM Patterns

### Model Managers

Each model has a manager that provides Django-like query methods:

```go
// Get or create pattern
user, created, err := db.Users.GetOrCreate("google", "123", models.User{
    Email: "new@example.com",
    Name:  "New User",
})

// Filter users
users, err := db.Users.Filter(map[string]interface{}{
    "email": "john@example.com",
})

// Get by ID
workspace, err := db.Workspaces.Get(workspaceID)

// Get by slug
workspace, err := db.Workspaces.GetBySlug("abc123")
```

### Instance Methods

Models have instance methods for common operations:

```go
// Workspace methods
members, err := workspace.GetMembers(db.DB)
isOwner, err := workspace.IsOwner(db.DB, userID)
err = workspace.AddMember(db.DB, user, models.RoleMember)
err = workspace.RemoveMember(db.DB, user)
err = workspace.UpdateMemberRole(db.DB, user, models.RoleAdmin)

// User methods
workspaces, err := user.GetActiveWorkspaces(db.DB)
workspace, err := user.CreatePersonalWorkspace(db.DB)
isMember, err := user.IsWorkspaceMember(db.DB, workspaceID)
role, err := user.GetWorkspaceRole(db.DB, workspaceID)

// Membership methods
err = membership.Activate(db.DB)
err = membership.Suspend(db.DB)
canManage := membership.CanManageMembers()
```

### Transactions

Use transactions for complex operations:

```go
err = db.Transaction(func(txDB *models.DB) error {
    // Create workspace
    workspace := &models.Workspace{Name: "New Workspace"}
    if err := txDB.Workspaces.Create(workspace); err != nil {
        return err
    }
    
    // Add owner
    return workspace.AddMember(txDB.DB, user, models.RoleOwner)
})
```

### Helper Functions

Django-like helper functions are available:

```go
// Check if object exists
exists, err := models.Exists[models.User](db.DB, "email = ?", "user@example.com")

// Count objects
count, err := models.Count[models.WorkspaceMembership](db.DB, "workspace_id = ?", workspaceID)

// Get object or 404
user, err := models.GetObjectOr404[models.User](db.DB, userID)

// Bulk create
users := []models.User{{...}, {...}, {...}}
err = models.BulkCreate(db.DB, users)
```

## Model Structure

### User
- Primary key: `ID` (int)
- Unique: `email`, `(provider, provider_id)`
- Has many: `Memberships`, `Workspaces` (through memberships)

### Workspace
- Primary key: `ID` (UUID)
- Unique: `slug` (6-character alphanumeric)
- Has many: `Memberships`, `Members` (through memberships)
- Auto-generates slug if not provided

### WorkspaceMembership
- Primary key: `ID` (UUID)
- Foreign keys: `WorkspaceID`, `UserID`, `InvitedBy`
- Enums: `Role` (owner/admin/member/viewer), `Status` (active/suspended/invited)

### WorkspaceInvitation
- Primary key: `ID` (UUID)
- Unique: `token`
- Foreign keys: `WorkspaceID`, `InviterID`, `InviteeID`
- Auto-generates secure token
- Default expiry: 7 days

## Enums

The package provides type-safe enums:

```go
// Workspace Plans
models.PlanFree
models.PlanPro
models.PlanEnterprise

// Membership Roles
models.RoleOwner
models.RoleAdmin
models.RoleMember
models.RoleViewer

// Membership Status
models.StatusActive
models.StatusSuspended
models.StatusInvited
```

## Migration Support

The package is designed to work with your existing migrations:

```go
// Run existing SQL migrations
adapter := models.NewMigrateAdapter(db.DB)
err = adapter.RunMigrations()

// Or use the backward-compatible service
service, err := models.NewDatabaseService()
```

## Business Rules

The models enforce these business rules:

1. **Workspace must have at least one owner** - Enforced when removing/changing owner role
2. **Unique workspace membership** - One membership per user per workspace
3. **Slug generation** - Auto-generates unique 6-character slug if not provided
4. **Invitation expiry** - Invitations expire after 7 days by default

## Example: Complete Workflow

```go
func CreateWorkspaceWithMembers(db *models.DB) error {
    return db.Transaction(func(txDB *models.DB) error {
        // 1. Create owner
        owner, _, err := txDB.Users.GetOrCreate("google", "123", models.User{
            Email: "owner@example.com",
            Name:  "Owner User",
        })
        if err != nil {
            return err
        }

        // 2. Create workspace
        workspace := &models.Workspace{
            Name:        "Team Workspace",
            Description: "Our team workspace",
            Plan:        models.PlanPro,
        }
        if err := txDB.Workspaces.Create(workspace); err != nil {
            return err
        }

        // 3. Add owner
        if err := workspace.AddMember(txDB.DB, owner, models.RoleOwner); err != nil {
            return err
        }

        // 4. Invite team members
        emails := []string{"member1@example.com", "member2@example.com"}
        for _, email := range emails {
            _, err := workspace.InviteMember(txDB.DB, owner.ID, email, models.RoleMember)
            if err != nil {
                return err
            }
        }

        return nil
    })
}
```

## Testing

The models are designed to be easily testable:

```go
func TestWorkspaceMembership(t *testing.T) {
    db, _ := models.NewDB()
    defer db.Close()

    // Use transactions for test isolation
    db.Transaction(func(txDB *models.DB) error {
        // Your test code here
        return errors.New("rollback") // Force rollback
    })
}
```