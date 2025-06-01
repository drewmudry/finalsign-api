package routes

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"finalsign/internal/database"
)

type Middleware struct {
	server ServerInterface
}

func NewMiddleware(server ServerInterface) *Middleware {
	return &Middleware{server: server}
}

func (m *Middleware) AuthMiddleware() gin.HandlerFunc {
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

		db := m.server.GetDB()
		user, err := db.GetUserByID(userID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "User not found or database error"})
			return
		}

		c.Set("user", user) // Store user object in context
		c.Next()
	}
}

// WorkspaceMiddleware checks if user has access to the workspace
func (m *Middleware) WorkspaceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User not found in context"})
			return
		}

		userObj := user.(*database.User)
		workspaceSlug := c.Param("slug")

		db := m.server.GetDB()
		userWorkspace, err := db.CheckUserWorkspaceAccess(userObj.ID, workspaceSlug)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Access denied to workspace"})
			return
		}

		c.Set("workspace", userWorkspace)
		c.Next()
	}
}