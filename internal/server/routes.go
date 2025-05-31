package server

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/markbates/goth/gothic"

	"finalsign/internal/auth"
	"finalsign/internal/database"
)

func (s *Server) RegisterRoutes() http.Handler {
	// Initialize Goth providers
	auth.InitGothProviders()

	r := gin.Default()

	// Set up sessions
	store := cookie.NewStore([]byte(os.Getenv("SESSION_SECRET")))
	r.Use(sessions.Sessions("finalsign-session", store))

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000", "https://finalsign.io", "https://www.finalsign.io"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	r.GET("/", s.HelloWorldHandler)
	r.GET("/health", s.healthHandler)

	// OAuth routes
	r.GET("/auth/:provider", s.authHandler)
	r.GET("/auth/:provider/callback", s.authCallbackHandler)
	r.GET("/logout", s.logoutHandler)
	r.GET("/user", s.AuthMiddleware(), s.userHandler)

	r.GET("/dashboard", s.AuthMiddleware(), s.DashboardHandler)

	// Workspace routes
	r.GET("/workspaces", s.AuthMiddleware(), s.GetUserWorkspacesHandler)
	r.GET("/workspaces/:slug", s.AuthMiddleware(), s.WorkspaceMiddleware(), s.GetWorkspaceHandler)
	r.POST("/workspaces/:slug/invite", s.AuthMiddleware(), s.WorkspaceMiddleware(), s.InviteToWorkspaceHandler)
	r.POST("/workspaces/:slug/accept-invite", s.AuthMiddleware(), s.AcceptWorkspaceInvitationHandler)

	return r
}

func (s *Server) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		userIDRaw := session.Get("user_id")

		if userIDRaw == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
			return
		}

		userID, ok := userIDRaw.(int)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid session data"})
			return
		}

		user, err := s.db.GetUserByID(userID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "User not found or database error"})
			return
		}

		c.Set("user", user) // Store user object in context
		c.Next()
	}
}

// WorkspaceMiddleware checks if user has access to the workspace
func (s *Server) WorkspaceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User not found in context"})
			return
		}

		userObj := user.(*database.User)
		workspaceSlug := c.Param("slug")

		userWorkspace, err := s.db.CheckUserWorkspaceAccess(userObj.ID, workspaceSlug)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Access denied to workspace"})
			return
		}

		c.Set("workspace", userWorkspace)
		c.Next()
	}
}

func (s *Server) HelloWorldHandler(c *gin.Context) {
	resp := make(map[string]string)
	resp["message"] = "Hello World"
	c.JSON(http.StatusOK, resp)
}

func (s *Server) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, s.db.Health())
}

func (s *Server) authHandler(c *gin.Context) {
	provider := c.Param("provider")

	req := c.Request.Clone(c.Request.Context())
	req.URL.Path = "/auth/" + provider

	q := req.URL.Query()
	q.Add("provider", provider)
	req.URL.RawQuery = q.Encode()

	gothic.BeginAuthHandler(c.Writer, req)
}

func (s *Server) authCallbackHandler(c *gin.Context) {
	provider := c.Param("provider")

	req := c.Request.Clone(c.Request.Context())
	req.URL.Path = "/auth/" + provider + "/callback"

	q := req.URL.Query()
	q.Add("provider", provider)
	req.URL.RawQuery = q.Encode()

	gothUser, err := gothic.CompleteUserAuth(c.Writer, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user := &database.User{
		Provider:   gothUser.Provider,
		ProviderID: gothUser.UserID,
		Email:      gothUser.Email,
		Name:       gothUser.Name,
		AvatarURL:  gothUser.AvatarURL,
	}

	err = s.db.CreateOrUpdateUser(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save user"})
		return
	}

	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Set("email", user.Email)
	session.Save()

	redirectURL := os.Getenv("FRONTEND_URL")
	if redirectURL == "" {
		redirectURL = "https://finalsign.io"
	}

	c.Redirect(http.StatusTemporaryRedirect, redirectURL+"/home")
}

func (s *Server) logoutHandler(c *gin.Context) {
    session := sessions.Default(c)
    session.Clear()
    session.Save()

    c.Redirect(http.StatusFound, "http://localhost:3000/")
}

func (s *Server) userHandler(c *gin.Context) {
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

	c.JSON(http.StatusOK, gin.H{
		"user_id":       user.ID,
		"email":         user.Email,
		"name":          user.Name,
		"avatar_url":    user.AvatarURL,
		"authenticated": true,
	})
}

func (s *Server) DashboardHandler(c *gin.Context) {
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

	// Get user's workspaces for the dashboard
	workspaces, err := s.db.GetUserWorkspaces(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch workspaces"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"email":         user.Email,
		"workspaces":    workspaces,
	})
}

// GetUserWorkspacesHandler returns all workspaces for the authenticated user
func (s *Server) GetUserWorkspacesHandler(c *gin.Context) {
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

	workspaces, err := s.db.GetUserWorkspaces(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch workspaces"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"workspaces": workspaces})
}

// GetWorkspaceHandler returns details for a specific workspace
func (s *Server) GetWorkspaceHandler(c *gin.Context) {
	workspace := c.MustGet("workspace").(*database.UserWorkspace)
	c.JSON(http.StatusOK, gin.H{"workspace": workspace})
}

// InviteToWorkspaceHandler invites a user to a workspace by email
func (s *Server) InviteToWorkspaceHandler(c *gin.Context) {
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

	err := s.db.InviteUserToWorkspace(workspace.WorkspaceID, req.Email, user.ID, req.Role)
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

// AcceptWorkspaceInvitationHandler accepts a workspace invitation
func (s *Server) AcceptWorkspaceInvitationHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspaceSlug := c.Param("slug")

	// Get workspace by slug
	workspace, err := s.db.GetWorkspaceBySlug(workspaceSlug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
		return
	}

	err = s.db.AcceptWorkspaceInvitation(user.ID, workspace.ID)
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
