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
	r.POST("/waitlist", s.WaitlistHandler)

	r.GET("/dashboard", s.AuthMiddleware(), s.DashboardHandler)

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
			// Log this error, as it indicates a problem with session data type
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid session data"})
			return
		}

		user, err := s.db.GetUserByID(userID)
		if err != nil {
			// This could be a database error or user not found
			// For security, treat as unauthenticated if user not found
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "User not found or database error"})
			return
		}

		c.Set("user", user) // Store user object in context
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

	// Create a new request with the correct path for gothic
	req := c.Request.Clone(c.Request.Context())
	req.URL.Path = "/auth/" + provider

	// Add the provider to the URL query params (gothic expects this)
	q := req.URL.Query()
	q.Add("provider", provider)
	req.URL.RawQuery = q.Encode()

	gothic.BeginAuthHandler(c.Writer, req)
}

func (s *Server) authCallbackHandler(c *gin.Context) {
	provider := c.Param("provider")

	// Create a new request with the correct path for gothic
	req := c.Request.Clone(c.Request.Context())
	req.URL.Path = "/auth/" + provider + "/callback"

	// Add the provider to the URL query params
	q := req.URL.Query()
	q.Add("provider", provider)
	req.URL.RawQuery = q.Encode()

	gothUser, err := gothic.CompleteUserAuth(c.Writer, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create or update user in database
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

	// Store user ID in session
	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Set("email", user.Email)
	session.Save()

	// Redirect to frontend or return success
	c.Redirect(http.StatusTemporaryRedirect, "http://localhost:3000/dashboard")
}

func (s *Server) logoutHandler(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()

	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

func (s *Server) userHandler(c *gin.Context) {
	userRaw, exists := c.Get("user")
	if !exists {
		// This should ideally not happen if middleware is correctly applied
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User not found in context"})
		return
	}

	user, ok := userRaw.(*database.User)
	if !ok {
		// This indicates a programming error (e.g. wrong type stored in context)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user type in context"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user_id": user.ID, "email": user.Email, "name": user.Name, "avatar_url": user.AvatarURL, "authenticated": true})
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

	c.JSON(http.StatusOK, gin.H{"email": user.Email, "authenticated": true})
}

func (s *Server) WaitlistHandler(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
		Name  string `json:"name"`
		Phone string `json:"phone"`
	}

	// Parse and validate request body
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create waitlist entry
	waitlist := &database.Waitlist{
		Email: req.Email,
		Name:  req.Name,
		Phone: req.Phone,
	}

	if err := s.db.CreateWaitlistEntry(waitlist); err != nil {
		// Check if it's a duplicate key error
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			c.JSON(http.StatusConflict, gin.H{"error": "Email already exists in waitlist"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add to waitlist"})
		return
	}

	// Return success response
	c.JSON(http.StatusCreated, gin.H{
		"message": "Successfully added to waitlist",
		"data":    waitlist,
	})
}
