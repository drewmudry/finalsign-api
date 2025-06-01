package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"finalsign/internal/database"
)

type UserRoutes struct {
	server ServerInterface
}

func NewUserRoutes(server ServerInterface) *UserRoutes {
	return &UserRoutes{server: server}
}

func (ur *UserRoutes) RegisterRoutes(r *gin.Engine) {
	middleware := NewMiddleware(ur.server)
	
	// User routes
	r.GET("/user", middleware.AuthMiddleware(), ur.userHandler)
}

func (ur *UserRoutes) userHandler(c *gin.Context) {
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
