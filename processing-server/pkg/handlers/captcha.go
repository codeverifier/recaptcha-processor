package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	recaptchaenterprise "cloud.google.com/go/recaptchaenterprise/v2/apiv1"
	recaptchaenterprisepb "cloud.google.com/go/recaptchaenterprise/v2/apiv1/recaptchaenterprisepb"
	chi "github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

const (
	// apiKeyHeader       = "x-api-key"
	// siteKeyHeader      = "x-site-key"
	captchaTokenHeader = "x-recaptcha-token"
)

type statusCodeGiver interface {
	StatusCode() int
}

type jsonError struct {
	Err error
}

type wrapErrorAsResp struct {
	Error jsonError
}

type errorResp struct {
	code    int
	message string
}

type emptyResp struct {
}

type CaptchaVerifyOptions struct {
	Threshold         float64
	EnterpriseEnabled bool
	// Enterprise related options
	GoogleProjectId string
	// non-Enterprise options
	GoogleApi string
}

type captchaPayloadReq struct {
	secret   string
	response string
}

type siteVerifyResp struct {
	success     bool      `json:"success"`               // whether this request was a valid reCAPTCHA token for your site
	challengeTS time.Time `json:"challenge_ts"`          // timestamp of the challenge load (ISO format yyyy-MM-dd'T'HH:mm:ssZZ)
	score       *float64  `json:"score,omitempty"`       // the score for this request (0.0 - 1.0)
	action      *string   `json:"action,omitempty"`      // the action name for this request (important to verify)
	hostname    string    `json:"hostname,omitempty"`    // the hostname of the site where the reCAPTCHA was solved
	errorCodes  []string  `json:"error-codes,omitempty"` // optional
}

type captchaOptionsWrapper struct {
	captchaOptions *CaptchaVerifyOptions
	log            *zap.Logger
}

type AuthState struct {
	State struct {
		SiteKey string `json:"x-site-key"` // site key
	} `json:"state"`
}

func HandleCaptcha(mux chi.Router, captchaOptions *CaptchaVerifyOptions, log *zap.Logger) {
	cw := &captchaOptionsWrapper{
		captchaOptions: captchaOptions,
		log:            log,
	}
	mux.Post("/captcha-verify", createAuthHandler(
		func(ctx context.Context, _ any) (*emptyResp, error) {
			// We are leaving response intact
			return &emptyResp{}, nil
		}, cw))
}

func createAuthHandler[Req, Res any](cb func(context.Context, Req) (Res, error), cw *captchaOptionsWrapper) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Req
		res, err := cb(r.Context(), req)
		if err != nil {
			if err, ok := err.(statusCodeGiver); ok {
				w.WriteHeader(err.StatusCode())
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
			writeJSON(w, wrapErrorAsResp{jsonError{err}})
			return
		}

		// The any() wrapper around responseBody is to avoid this error:
		// "cannot use type assertion on type parameter value responseBody (variable of type Res constrained by any)"
		// See https://github.com/golang/go/issues/45380#issuecomment-1014950980
		if res, ok := any(res).(statusCodeGiver); ok {
			w.WriteHeader(res.StatusCode())
		} else {
			var authState AuthState
			decoder := json.NewDecoder(r.Body)
			decoderErr := decoder.Decode(&authState)
			defer r.Body.Close()
			if decoderErr != nil {
				cw.log.Error("error reading auth state body", zap.Error(decoderErr))
				http.Error(w, decoderErr.Error(), http.StatusUnauthorized)
				return
			}

			token := r.Header.Get(captchaTokenHeader)
			if authState.State.SiteKey != "" && token != "" {
				if err != nil {
					cw.log.Error("unable to decode site key")
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				errorResp, err := cw.createRecaptchaRequest(r.Context(), string(authState.State.SiteKey), token)
				if errorResp != nil {
					cw.log.Error(errorResp.message)
					http.Error(w, errorResp.message, errorResp.code)
					return
				} else if err != nil {
					cw.log.Error("site verification failure", zap.Error(err))
					http.Error(w, "site verification failure", http.StatusUnauthorized)
					return
				}
				cw.log.Info("successfully submitted and verified captcha")
				w.WriteHeader(http.StatusOK)
			} else {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		writeJSON(w, res)
	}
}

func (cw *captchaOptionsWrapper) createRecaptchaRequest(ctx context.Context, siteKey string, token string) (*errorResp, error) {
	if cw.captchaOptions.EnterpriseEnabled {
		c, err := recaptchaenterprise.NewClient(ctx)
		if err != nil {
			log.Fatal("unable to create recaptcha enterprise client", zap.Error(err))
		}
		defer c.Close()

		req := &recaptchaenterprisepb.CreateAssessmentRequest{
			// See https://pkg.go.dev/cloud.google.com/go/recaptchaenterprise/v2/apiv1/recaptchaenterprisepb#CreateAssessmentRequest
			Parent: fmt.Sprintf("projects/%s", cw.captchaOptions.GoogleProjectId),
			Assessment: &recaptchaenterprisepb.Assessment{
				Event: &recaptchaenterprisepb.Event{
					Token:   token,
					SiteKey: siteKey,
				},
			},
		}
		resp, err := c.CreateAssessment(ctx, req)
		if err != nil {
			log.Fatal("unable to process the recaptcha enterprise response", zap.Error(err))
		}
		return nil, cw.confirmEnterpriseAssessment(resp)
	} else {
		var captchaPayloadReq http.Request
		var siteVerifyResp siteVerifyResp
		captchaPayloadReq.ParseForm()
		captchaPayloadReq.Form.Add("secret", siteKey)
		captchaPayloadReq.Form.Add("response", token)

		verifyCaptchaResp, err := http.Post(cw.captchaOptions.GoogleApi, "application/x-www-form-urlencoded", strings.NewReader(captchaPayloadReq.Form.Encode()))
		if err != nil {
			return &errorResp{
				code:    http.StatusUnauthorized,
				message: "unable to solve captcha",
			}, nil
		}

		decoder := json.NewDecoder(verifyCaptchaResp.Body)
		decoderErr := decoder.Decode(&siteVerifyResp)

		defer verifyCaptchaResp.Body.Close()

		if decoderErr != nil {
			return &errorResp{
				code:    http.StatusUnauthorized,
				message: "unable to parse the response from captcha verification",
			}, nil
		}
		return nil, cw.confirm(siteVerifyResp)
	}
}

// Managing response from reCAPTCHA
func (cw *captchaOptionsWrapper) confirm(resp siteVerifyResp) error {
	if resp.errorCodes != nil {
		return fmt.Errorf("remote error codes: %v", resp.errorCodes)
	}

	if !resp.success {
		return fmt.Errorf("invalid challenge solution")
	}

	if resp.score == nil {
		return fmt.Errorf("no risk score available")
	}

	threshold := 0.0
	if cw.captchaOptions.Threshold != 0 {
		threshold = cw.captchaOptions.Threshold
	}
	if threshold >= *resp.score {
		return fmt.Errorf("received score '%f', while expecting minimum '%f'", *resp.score, threshold)
	}
	return nil
}

// Managing assessment from reCAPTCHA Enterprise
func (cw *captchaOptionsWrapper) confirmEnterpriseAssessment(resp *recaptchaenterprisepb.Assessment) error {
	if !resp.GetTokenProperties().GetValid() {
		return fmt.Errorf("token is invalid: '%d'", int(resp.GetTokenProperties().GetInvalidReason()))
	}

	threshold := 0.0
	if cw.captchaOptions.Threshold != 0 {
		threshold = cw.captchaOptions.Threshold
	}
	if threshold >= float64(resp.GetRiskAnalysis().GetScore()) {
		return fmt.Errorf("received score '%f', while expecting minimum '%f'", float64(resp.GetRiskAnalysis().GetScore()), threshold)
	}
	return nil
}

func writeJSON(w io.Writer, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic(err)
	}
}
