package routes

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"finalsign/internal/database"
)

type NotificationRoutes struct {
	server ServerInterface
}

func NewNotificationRoutes(server ServerInterface) *NotificationRoutes {
	return &NotificationRoutes{server: server}
}

func (nr *NotificationRoutes) RegisterRoutes(r *gin.Engine) {
	// Create middleware instance
	middleware := NewMiddleware(nr.server)
	
	// Notification routes
	r.GET("/notifications", middleware.AuthMiddleware(), nr.getUserNotificationsHandler)
	r.POST("/notifications/:id/read", middleware.AuthMiddleware(), nr.markNotificationAsReadHandler)
}

// getUserNotificationsHandler returns all notifications for the authenticated user
func (nr *NotificationRoutes) getUserNotificationsHandler(c *gin.Context) {
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

	// Get limit from query parameter, default to 50
	limitStr := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 50
	}
	
	// Cap the limit to prevent excessive queries
	if limit > 100 {
		limit = 100
	}

	db := nr.server.GetDB()
	notifications, err := db.GetUserNotifications(user.ID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notifications"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"notifications": notifications})
}

// markNotificationAsReadHandler marks a specific notification as read
func (nr *NotificationRoutes) markNotificationAsReadHandler(c *gin.Context) {
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

	// Parse notification ID from URL parameter
	notificationIDStr := c.Param("id")
	notificationID, err := uuid.Parse(notificationIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid notification ID"})
		return
	}

	db := nr.server.GetDB()
	err = db.MarkNotificationAsRead(notificationID, user.ID)
	if err != nil {
		if err.Error() == "notification not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark notification as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notification marked as read"})
}