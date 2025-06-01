package routes

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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
	
	// Existing workspace routes
	r.GET("/workspaces", middleware.AuthMiddleware(), wr.getUserWorkspacesHandler)
	r.GET("/workspaces/:slug", middleware.AuthMiddleware(), middleware.WorkspaceMiddleware(), wr.getWorkspaceHandler)
	r.POST("/workspaces/:slug/invite", middleware.AuthMiddleware(), middleware.WorkspaceMiddleware(), wr.inviteToWorkspaceHandler)
	r.POST("/invitations/:token/accept", middleware.AuthMiddleware(), wr.acceptWorkspaceInvitationHandler)
	r.POST("/invitations/:token/decline", middleware.AuthMiddleware(), wr.declineWorkspaceInvitationHandler)

	// NEW: Enhanced workspace management routes
	r.PUT("/workspaces/:slug", middleware.AuthMiddleware(), middleware.WorkspaceMiddleware(), wr.updateWorkspaceHandler)
	r.GET("/workspaces/:slug/members", middleware.AuthMiddleware(), middleware.WorkspaceMiddleware(), wr.getWorkspaceMembersHandler)
	r.GET("/workspaces/:slug/invitations", middleware.AuthMiddleware(), middleware.WorkspaceMiddleware(), wr.getWorkspacePendingInvitationsHandler)
	r.PUT("/workspaces/:slug/members/:userID/role", middleware.AuthMiddleware(), middleware.WorkspaceMiddleware(), wr.updateMemberRoleHandler)
	r.DELETE("/workspaces/:slug/members/:userID", middleware.AuthMiddleware(), middleware.WorkspaceMiddleware(), wr.removeMemberHandler)
	r.DELETE("/workspaces/:slug/invitations/:invitationID", middleware.AuthMiddleware(), middleware.WorkspaceMiddleware(), wr.cancelInvitationHandler)
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
	userWorkspace := c.MustGet("workspace").(*database.UserWorkspace)

	// Check if user has permission to invite (owner or admin)
	if userWorkspace.Role != "owner" && userWorkspace.Role != "admin" {
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
	err := db.InviteUserToWorkspace(userWorkspace.WorkspaceID, req.Email, user.ID, req.Role)
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
	token := c.Param("token")

	db := wr.server.GetDB()

	err := db.AcceptWorkspaceInvitationByToken(token, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "invalid or expired invitation") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Invalid or expired invitation"})
			return
		}
		if strings.Contains(err.Error(), "invitation is not for this user") {
			c.JSON(http.StatusForbidden, gin.H{"error": "This invitation is not for you"})
			return
		}
		if strings.Contains(err.Error(), "already processed") {
			c.JSON(http.StatusConflict, gin.H{"error": "Invitation has already been processed"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to accept invitation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Invitation accepted successfully"})
}

// declineWorkspaceInvitationHandler declines a workspace invitation
func (wr *WorkspaceRoutes) declineWorkspaceInvitationHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	token := c.Param("token")

	db := wr.server.GetDB()

	err := db.DeclineWorkspaceInvitation(token, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "invalid or expired invitation") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Invalid or expired invitation"})
			return
		}
		if strings.Contains(err.Error(), "invitation is not for this user") {
			c.JSON(http.StatusForbidden, gin.H{"error": "This invitation is not for you"})
			return
		}
		if strings.Contains(err.Error(), "already processed") {
			c.JSON(http.StatusConflict, gin.H{"error": "Invitation has already been processed"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decline invitation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Invitation declined successfully"})
}

func (wr *WorkspaceRoutes) updateWorkspaceHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspace := c.MustGet("workspace").(*database.UserWorkspace)

	var req struct {
		Name        string `json:"name" binding:"required,min=1,max=100"`
		Description string `json:"description" binding:"max=500"`
		Settings    struct {
			Theme       string `json:"theme"`       // e.g., "light", "dark", "blue"
			Color       string `json:"color"`       // primary color
			Logo        string `json:"logo"`        // logo URL
			CustomCSS   string `json:"custom_css"`  // custom styling
			Timezone    string `json:"timezone"`    // workspace timezone
			Language    string `json:"language"`    // workspace language
		} `json:"settings"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate theme if provided
	if req.Settings.Theme != "" {
		validThemes := map[string]bool{"light": true, "dark": true, "blue": true, "green": true, "purple": true}
		if !validThemes[req.Settings.Theme] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid theme"})
			return
		}
	}

	// Convert settings to JSON using proper JSON marshaling
	settingsMap := map[string]string{
		"theme":      req.Settings.Theme,
		"color":      req.Settings.Color,
		"logo":       req.Settings.Logo,
		"custom_css": req.Settings.CustomCSS,
		"timezone":   req.Settings.Timezone,
		"language":   req.Settings.Language,
	}

	settingsBytes, err := json.Marshal(settingsMap)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process settings"})
		return
	}
	settingsJSON := string(settingsBytes)

	db := wr.server.GetDB()
	err = db.UpdateWorkspace(workspace.WorkspaceID, req.Name, req.Description, settingsJSON, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "insufficient permissions") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to update workspace"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update workspace"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Workspace updated successfully"})
}

// getWorkspaceMembersHandler returns all members of a workspace
func (wr *WorkspaceRoutes) getWorkspaceMembersHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspace := c.MustGet("workspace").(*database.UserWorkspace)

	db := wr.server.GetDB()
	members, err := db.GetWorkspaceMembers(workspace.WorkspaceID, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch workspace members"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"members": members,
		"total": len(members),
	})
}

// getWorkspacePendingInvitationsHandler returns pending invitations for a workspace
func (wr *WorkspaceRoutes) getWorkspacePendingInvitationsHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspace := c.MustGet("workspace").(*database.UserWorkspace)

	db := wr.server.GetDB()
	invitations, err := db.GetWorkspacePendingInvitations(workspace.WorkspaceID, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "insufficient permissions") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to view pending invitations"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pending invitations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"invitations": invitations,
		"total": len(invitations),
	})
}

// updateMemberRoleHandler updates a member's role
func (wr *WorkspaceRoutes) updateMemberRoleHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspace := c.MustGet("workspace").(*database.UserWorkspace)
	
	memberUserIDStr := c.Param("userID")
	memberUserID, err := strconv.Atoi(memberUserIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req struct {
		Role string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate role
	validRoles := map[string]bool{"owner": true, "admin": true, "member": true, "viewer": true}
	if !validRoles[req.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role. Must be owner, admin, member, or viewer"})
		return
	}

	db := wr.server.GetDB()
	err = db.UpdateMemberRole(workspace.WorkspaceID, memberUserID, req.Role, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "insufficient permissions") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to update member roles"})
			return
		}
		if strings.Contains(err.Error(), "only workspace owners") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Only workspace owners can manage owner roles"})
			return
		}
		if strings.Contains(err.Error(), "member not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Member not found in workspace"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update member role"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Member role updated successfully"})
}

// removeMemberHandler removes a member from the workspace
func (wr *WorkspaceRoutes) removeMemberHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspace := c.MustGet("workspace").(*database.UserWorkspace)
	
	memberUserIDStr := c.Param("userID")
	memberUserID, err := strconv.Atoi(memberUserIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	db := wr.server.GetDB()
	err = db.RemoveMemberFromWorkspace(workspace.WorkspaceID, memberUserID, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "insufficient permissions") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to remove members"})
			return
		}
		if strings.Contains(err.Error(), "only workspace owners can remove") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Only workspace owners can remove other owners"})
			return
		}
		if strings.Contains(err.Error(), "cannot remove the last owner") {
			c.JSON(http.StatusConflict, gin.H{"error": "Cannot remove the last owner from workspace"})
			return
		}
		if strings.Contains(err.Error(), "member not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Member not found in workspace"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove member"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Member removed successfully"})
}

// cancelInvitationHandler cancels a pending invitation
func (wr *WorkspaceRoutes) cancelInvitationHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	
	invitationIDStr := c.Param("invitationID")
	invitationID, err := uuid.Parse(invitationIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invitation ID"})
		return
	}

	db := wr.server.GetDB()
	err = db.CancelWorkspaceInvitation(invitationID, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "insufficient permissions") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to cancel invitation"})
			return
		}
		if strings.Contains(err.Error(), "invitation not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found or already processed"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel invitation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Invitation cancelled successfully"})
}

// Enhanced getWorkspaceHandler with detailed info
func (wr *WorkspaceRoutes) getWorkspaceDetailedHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspace := c.MustGet("workspace").(*database.UserWorkspace)

	db := wr.server.GetDB()
	
	// Get members count
	members, err := db.GetWorkspaceMembers(workspace.WorkspaceID, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch workspace details"})
		return
	}

	// Get pending invitations count (only for owners/admins)
	var pendingInvitations []database.WorkspaceInvitation
	if workspace.Role == "owner" || workspace.Role == "admin" {
		pendingInvitations, _ = db.GetWorkspacePendingInvitations(workspace.WorkspaceID, user.ID)
	}

	// Parse settings JSON for easier consumption
	// var settings map[string]interface{}
	if workspace.Plan != "" {
		// You might want to fetch full workspace details here including settings
		// For now, we'll return the workspace as-is
	}

	c.JSON(http.StatusOK, gin.H{
		"workspace": workspace,
		"stats": gin.H{
			"members_count": len(members),
			"pending_invitations_count": len(pendingInvitations),
		},
		"permissions": gin.H{
			"can_invite": workspace.Role == "owner" || workspace.Role == "admin",
			"can_edit": workspace.Role == "owner" || workspace.Role == "admin",
			"can_manage_members": workspace.Role == "owner" || workspace.Role == "admin",
		},
	})
}