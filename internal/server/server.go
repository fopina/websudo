package server

import (
	"context"

	"github.com/fopina/websudo/internal/config"
)

// Server is a lightweight placeholder for the future reverse proxy runtime.
type Server struct {
	cfg *config.Config
}

// New creates a server from config.
func New(cfg *config.Config) *Server {
	return &Server{cfg: cfg}
}

// Run starts the server. For the initial scaffold, it validates wiring only.
func (s *Server) Run(_ context.Context) error {
	_ = s.cfg
	return nil
}
