package server

import "github.com/pseudonator/recaptcha-processing-server/pkg/handlers"

func (s *Server) setupRoutes() {
	handlers.Health(s.mux)
	handlers.HandleCaptcha(s.mux, s.captcha, s.log)
}
