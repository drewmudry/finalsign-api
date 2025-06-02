package routes

import (
	"encoding/json"
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
	middleware := NewMiddleware(nr.server)
	r.GET("/notifications", middleware.AuthMiddleware(), nr.getUserNotificationsHandler)
	r.POST("/notifications/:id/read", middleware.AuthMiddleware(), nr.markNotificationAsReadHandler)
	r.POST("/notifications/:id/accept", middleware.AuthMiddleware(), nr.acceptInvitationNotificationHandler)
	r.POST("/notifications/:id/decline", middleware.AuthMiddleware(), nr.declineInvitationNotificationHandler)
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

// acceptInvitationNotificationHandler accepts an invitation from a notification
func (nr *NotificationRoutes) acceptInvitationNotificationHandler(c *gin.Context) {
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
	
	// Get the notification first to extract invitation data
	notifications, err := db.GetUserNotifications(user.ID, 100) // Get all to find the specific one
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notification"})
		return
	}

	var targetNotification *database.Notification
	for _, notification := range notifications {
		if notification.ID == notificationID {
			targetNotification = notification
			break
		}
	}

	if targetNotification == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
		return
	}

	// Check if this is a workspace invitation notification
	if targetNotification.Type != "workspace_invitation" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "This notification is not an invitation"})
		return
	}

	// Parse the invitation data from the notification
	var invitationData struct {
		InvitationID string `json:"invitation_id"`
		Token        string `json:"token"`
	}

	err = json.Unmarshal([]byte(targetNotification.Data), &invitationData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid invitation data"})
		return
	}

	// Accept the invitation using the token
	err = db.AcceptWorkspaceInvitationByToken(invitationData.Token, user.ID)
	if err != nil {
		if err.Error() == "invalid or expired invitation" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Invalid or expired invitation"})
			return
		}
		if err.Error() == "invitation is not for this user" {
			c.JSON(http.StatusForbidden, gin.H{"error": "This invitation is not for you"})
			return
		}
		if err.Error() == "invitation not found or already processed" {
			c.JSON(http.StatusConflict, gin.H{"error": "Invitation has already been processed"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to accept invitation"})
		return
	}

	// Mark the notification as read
	db.MarkNotificationAsRead(notificationID, user.ID)

	c.JSON(http.StatusOK, gin.H{"message": "Invitation accepted successfully"})
}

// declineInvitationNotificationHandler declines an invitation from a notification
func (nr *NotificationRoutes) declineInvitationNotificationHandler(c *gin.Context) {
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
	
	// Get the notification first to extract invitation data
	notifications, err := db.GetUserNotifications(user.ID, 100) // Get all to find the specific one
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notification"})
		return
	}

	var targetNotification *database.Notification
	for _, notification := range notifications {
		if notification.ID == notificationID {
			targetNotification = notification
			break
		}
	}

	if targetNotification == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
		return
	}

	// Check if this is a workspace invitation notification
	if targetNotification.Type != "workspace_invitation" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "This notification is not an invitation"})
		return
	}

	// Parse the invitation data from the notification
	var invitationData struct {
		InvitationID string `json:"invitation_id"`
		Token        string `json:"token"`
	}

	err = json.Unmarshal([]byte(targetNotification.Data), &invitationData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid invitation data"})
		return
	}

	// Decline the invitation using the token
	err = db.DeclineWorkspaceInvitation(invitationData.Token, user.ID)
	if err != nil {
		if err.Error() == "invalid or expired invitation" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Invalid or expired invitation"})
			return
		}
		if err.Error() == "invitation is not for this user" {
			c.JSON(http.StatusForbidden, gin.H{"error": "This invitation is not for you"})
			return
		}
		if err.Error() == "invitation not found or already processed" {
			c.JSON(http.StatusConflict, gin.H{"error": "Invitation has already been processed"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decline invitation"})
		return
	}

	// Mark the notification as read
	db.MarkNotificationAsRead(notificationID, user.ID)

	c.JSON(http.StatusOK, gin.H{"message": "Invitation declined successfully"})
}