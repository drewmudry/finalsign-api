package server

import (
	"net/http"
	"os"

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
		AllowOrigins:     []string{"http://localhost:3000"},
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
	r.GET("/user", s.userHandler)

	r.GET("/dashboard", s.DashboardHandler)

	return r
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
	session := sessions.Default(c)
	userID := session.Get("user_id")

	if userID == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	// You could fetch full user details from database here
	c.JSON(http.StatusOK, gin.H{"user_id": userID, "authenticated": true})
}

func (s *Server) DashboardHandler(c *gin.Context) {
	session := sessions.Default(c)
	user_email := session.Get("email")

	if user_email == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	// You could fetch full user details from database here
	c.JSON(http.StatusOK, gin.H{"email": user_email, "authenticated": true})
}
