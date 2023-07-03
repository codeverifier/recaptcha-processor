package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	chi "github.com/go-chi/chi/v5"
	captcha "github.com/pseudonator/recaptcha-processing-server/pkg/handlers"
	"go.uber.org/zap"
)

const (
	defaultServerHost                 = "localhost"
	defaultServerPort                 = 8090
	defaultThreshold                  = 0.5
	defaultRecaptchaEnterpriseEnabled = true
)

type Options struct {
	Log *zap.Logger
}

type Server struct {
	address string
	log     *zap.Logger
	mux     chi.Router
	server  *http.Server
	captcha *captcha.CaptchaVerifyOptions
}

type loggerWrapper struct {
	log *zap.Logger
}

func New(opts Options) *Server {
	if opts.Log == nil {
		opts.Log = zap.NewNop()
	}
	log := opts.Log

	lw := &loggerWrapper{log: log}

	address := lw.getBindingAddress()
	mux := chi.NewMux()
	return &Server{
		address: address,
		log:     log,
		mux:     mux,
		server: &http.Server{
			Addr:              address,
			Handler:           mux,
			ReadTimeout:       5 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
			WriteTimeout:      5 * time.Second,
			IdleTimeout:       5 * time.Second,
		},
		captcha: lw.getCaptchaVerify(),
	}
}

func (lw *loggerWrapper) getBindingAddress() string {
	host := lw.getStringOrDefault("SERVER_HOST", defaultServerHost)
	port := lw.getIntOrDefault("SERVER_PORT", defaultServerPort)
	address := net.JoinHostPort(host, strconv.Itoa(port))
	return address
}

func (lw *loggerWrapper) getCaptchaVerify() *captcha.CaptchaVerifyOptions {
	captchaOptions := &captcha.CaptchaVerifyOptions{}
	isEnterprise := lw.getBoolOrDefault("ENABLE_ENTERPRISE", defaultRecaptchaEnterpriseEnabled)
	captchaOptions.EnterpriseEnabled = isEnterprise
	if isEnterprise {
		// For authenticating on GC
		// https://cloud.google.com/recaptcha-enterprise/docs/set-up-non-google-cloud-environments
		// https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity
		captchaOptions.GoogleProjectId = lw.getEnvVarOrError("GOOGLE_ADC_PROJECT_ID")
		// https://cloud.google.com/recaptcha-enterprise/docs/create-assessment#create_API_client_libraries
		captchaOptions.SiteKey = lw.getEnvVarOrError("CAPTCHA_SITE_KEY")
	} else {
		captchaOptions.GoogleApi = lw.getEnvVarOrError("VERIFY_CAPTCHA_GOOGLE_API")
		// https://developers.google.com/recaptcha/docs/verify#api_request
		captchaOptions.SharedKey = lw.getEnvVarOrError("CAPTCHA_SHARED_KEY")
	}
	captchaOptions.Threshold = lw.getFloatOrDefault("ACCEPTABLE_SCORE_THRESHOLD", defaultThreshold)
	return captchaOptions
}

// Start the server by setting up routes and listening for HTTP requests on the given address.
func (s *Server) Start() error {
	s.setupRoutes()

	s.log.Info("starting", zap.String("address", s.address))
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("error starting server: %w", err)
	}
	return nil
}

// Stop the server gracefully within the timeout.
func (s *Server) Stop() error {
	s.log.Info("stopping server")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("error stopping server: %w", err)
	}

	return nil
}

func (lw *loggerWrapper) getBoolOrDefault(name string, defaultV bool) bool {
	v, ok := os.LookupEnv(name)
	if !ok {
		return defaultV
	}
	vAsBool, err := strconv.ParseBool(v)
	if err != nil {
		return defaultV
	}
	return vAsBool
}

func (lw *loggerWrapper) getStringOrDefault(name string, defaultV string) string {
	v, ok := os.LookupEnv(name)
	if !ok {
		return defaultV
	}
	return strings.TrimSpace(v)
}

func (lw *loggerWrapper) getFloatOrDefault(name string, defaultV float64) float64 {
	v, ok := os.LookupEnv(name)
	if !ok {
		return defaultV
	}
	vAsFloat, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultV
	}
	return vAsFloat
}

func (lw *loggerWrapper) getIntOrDefault(name string, defaultV int) int {
	v, ok := os.LookupEnv(name)
	if !ok {
		return defaultV
	}
	vAsInt, err := strconv.Atoi(v)
	if err != nil {
		return defaultV
	}
	return vAsInt
}

func (lw *loggerWrapper) getEnvVarOrError(name string) string {
	v, ok := os.LookupEnv(name)
	if !ok {
		panic(errors.New(fmt.Sprintf("given env var is not present, %s not found !", name)))
	}
	return v
}
