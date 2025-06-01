package routes

import (
	"net/http"
	"os"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/markbates/goth/gothic"

	"finalsign/internal/database"
)

type AuthRoutes struct {
	server ServerInterface
}

type ServerInterface interface {
	GetDB() database.Service
}

func NewAuthRoutes(server ServerInterface) *AuthRoutes {
	return &AuthRoutes{server: server}
}

func (ar *AuthRoutes) RegisterRoutes(r *gin.Engine) {
	// OAuth routes
	r.GET("/auth/:provider", ar.authHandler)
	r.GET("/auth/:provider/callback", ar.authCallbackHandler)
	r.GET("/logout", ar.logoutHandler)
}

func (ar *AuthRoutes) authHandler(c *gin.Context) {
	provider := c.Param("provider")

	req := c.Request.Clone(c.Request.Context())
	req.URL.Path = "/auth/" + provider

	q := req.URL.Query()
	q.Add("provider", provider)
	req.URL.RawQuery = q.Encode()

	gothic.BeginAuthHandler(c.Writer, req)
}

func (ar *AuthRoutes) authCallbackHandler(c *gin.Context) {
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

	db := ar.server.GetDB()
	err = db.CreateOrUpdateUser(user)
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

func (ar *AuthRoutes) logoutHandler(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()

	c.Redirect(http.StatusFound, "http://localhost:3000/")
}