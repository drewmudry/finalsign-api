package routes

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"finalsign/internal/database"
)

type WorkspaceRoutes struct {
	server ServerInterface
}

func NewWorkspaceRoutes(server ServerInterface) *WorkspaceRoutes {
	return &WorkspaceRoutes{server: server}
}

func (wr *WorkspaceRoutes) RegisterRoutes(r *gin.Engine) {
	// Create middleware instance
	middleware := NewMiddleware(wr.server)
	
	// Workspace routes
	r.GET("/workspaces", middleware.AuthMiddleware(), wr.getUserWorkspacesHandler)
	r.GET("/workspaces/:slug", middleware.AuthMiddleware(), middleware.WorkspaceMiddleware(), wr.getWorkspaceHandler)
	r.POST("/workspaces/:slug/invite", middleware.AuthMiddleware(), middleware.WorkspaceMiddleware(), wr.inviteToWorkspaceHandler)
	r.POST("/workspaces/:slug/accept-invite", middleware.AuthMiddleware(), wr.acceptWorkspaceInvitationHandler)
}

// getUserWorkspacesHandler returns all workspaces for the authenticated user
func (wr *WorkspaceRoutes) getUserWorkspacesHandler(c *gin.Context) {
	userRaw, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User not found in context"})
		return
	}

	user, ok := userRaw.(*database.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user type in context"})
		return
	}

	db := wr.server.GetDB()
	workspaces, err := db.GetUserWorkspaces(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch workspaces"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"workspaces": workspaces})
}

// getWorkspaceHandler returns details for a specific workspace
func (wr *WorkspaceRoutes) getWorkspaceHandler(c *gin.Context) {
	workspace := c.MustGet("workspace").(*database.UserWorkspace)
	c.JSON(http.StatusOK, gin.H{"workspace": workspace})
}

// inviteToWorkspaceHandler invites a user to a workspace by email
func (wr *WorkspaceRoutes) inviteToWorkspaceHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspace := c.MustGet("workspace").(*database.UserWorkspace)

	// Check if user has permission to invite (owner or admin)
	if workspace.Role != "owner" && workspace.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to invite users"})
		return
	}

	var req struct {
		Email string `json:"email" binding:"required,email"`
		Role  string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate role
	validRoles := map[string]bool{"member": true, "admin": true, "viewer": true}
	if !validRoles[req.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role. Must be member, admin, or viewer"})
		return
	}

	db := wr.server.GetDB()
	err := db.InviteUserToWorkspace(workspace.WorkspaceID, req.Email, user.ID, req.Role)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "User with that email not found"})
			return
		}
		if strings.Contains(err.Error(), "already a member") {
			c.JSON(http.StatusConflict, gin.H{"error": "User is already a member of this workspace"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send invitation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Invitation sent successfully"})
}

// acceptWorkspaceInvitationHandler accepts a workspace invitation
func (wr *WorkspaceRoutes) acceptWorkspaceInvitationHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspaceSlug := c.Param("slug")

	db := wr.server.GetDB()
	
	// Get workspace by slug
	workspace, err := db.GetWorkspaceBySlug(workspaceSlug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
		return
	}

	err = db.AcceptWorkspaceInvitation(user.ID, workspace.ID)
	if err != nil {
		if strings.Contains(err.Error(), "no pending invitation") {
			c.JSON(http.StatusNotFound, gin.H{"error": "No pending invitation found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to accept invitation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Invitation accepted successfully"})
}