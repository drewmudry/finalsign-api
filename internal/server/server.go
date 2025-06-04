package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"

	"finalsign/internal/database"
	"finalsign/internal/storage"
)

type Server struct {
	port      int
	db        database.Service
	s3Service *storage.S3Service
}

func (s *Server) GetDB() database.Service {
	return s.db
}

func (s *Server) GetS3Service() *storage.S3Service {
	return s.s3Service
}

func NewServer() *http.Server {
	port, _ := strconv.Atoi(os.Getenv("PORT"))
	s3Service, err := storage.NewS3Service()
	if err != nil {
		log.Fatalf("Failed to initialize S3 service: %v", err)
	}

	NewServer := &Server{
		port:      port,
		db:        database.New(),
		s3Service: s3Service,
	}

	// Declare Server config
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", NewServer.port),
		Handler:      NewServer.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return server
}
