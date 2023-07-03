package server

import (
	"github.com/pseudonator/recaptcha-test-client/pkg/handlers"
)

func (s *Server) setupRoutes() {
	handlers.Render(s.mux, s.log)
}
