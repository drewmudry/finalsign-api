package server

import (
	"net/http"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"finalsign/internal/auth"
	"finalsign/internal/server/routes"
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

	r.GET("/health", s.healthHandler)

	// Initialize route handlers
	authRoutes := routes.NewAuthRoutes(s)
	userRoutes := routes.NewUserRoutes(s)
	workspaceRoutes := routes.NewWorkspaceRoutes(s)

	// Register route groups
	authRoutes.RegisterRoutes(r)
	userRoutes.RegisterRoutes(r)
	workspaceRoutes.RegisterRoutes(r)

	return r
}


func (s *Server) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, s.db.Health())
}