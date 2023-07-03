package server

import (
	"github.com/pseudonator/recaptcha-test-backend-server/pkg/handlers"
)

func (s *Server) setupRoutes() {
	handlers.SubmitUser(s.mux, s.log)
}
