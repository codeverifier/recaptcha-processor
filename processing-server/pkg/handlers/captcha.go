package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	recaptchaenterprise "cloud.google.com/go/recaptchaenterprise/v2/apiv1"
	recaptchaenterprisepb "cloud.google.com/go/recaptchaenterprise/v2/apiv1/recaptchaenterprisepb"
	chi "github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

const (
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

type CaptchaVerifyOptions struct {
	Threshold         float64
	EnterpriseEnabled bool
	// Enterprise related options
	GoogleProjectId string
	SiteKey         string
	// non-Enterprise options
	GoogleApi string
	SharedKey string
}

type emptyResp struct {
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

func HandleCaptcha(mux chi.Router, captchaOptions *CaptchaVerifyOptions, log *zap.Logger) {
	mux.Post("/captcha-verify", createAuthHandler(
		func(ctx context.Context, _ any) (*emptyResp, error) {
			// We are leaving response intact
			return &emptyResp{}, nil
		}, captchaOptions, log))
}

func createRecaptchaRequest(ctx context.Context, token string, captchaOptions *CaptchaVerifyOptions, log *zap.Logger) (*errorResp, error) {
	if captchaOptions.EnterpriseEnabled {
		c, err := recaptchaenterprise.NewClient(ctx)
		if err != nil {
			log.Fatal("unable to create recaptcha enterprise client", zap.Error(err))
		}
		defer c.Close()

		req := &recaptchaenterprisepb.CreateAssessmentRequest{
			// See https://pkg.go.dev/cloud.google.com/go/recaptchaenterprise/v2/apiv1/recaptchaenterprisepb#CreateAssessmentRequest
			Parent: fmt.Sprintf("projects/%s", captchaOptions.GoogleProjectId),
			Assessment: &recaptchaenterprisepb.Assessment{
				Event: &recaptchaenterprisepb.Event{
					Token:   token,
					SiteKey: captchaOptions.SiteKey,
				},
			},
		}
		resp, err := c.CreateAssessment(ctx, req)
		if err != nil {
			log.Fatal("unable to process the recaptcha enterprise response", zap.Error(err))
		}
		return nil, confirmEnterpriseAssessment(resp, captchaOptions)
	} else {
		var captchaPayloadReq http.Request
		var siteVerifyResp siteVerifyResp
		captchaPayloadReq.ParseForm()
		captchaPayloadReq.Form.Add("secret", captchaOptions.SharedKey)
		captchaPayloadReq.Form.Add("response", token)

		verifyCaptchaResp, err := http.Post(captchaOptions.GoogleApi, "application/x-www-form-urlencoded", strings.NewReader(captchaPayloadReq.Form.Encode()))
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
		return nil, confirm(siteVerifyResp, captchaOptions)
	}
}

func createAuthHandler[Req, Res any](cb func(context.Context, Req) (Res, error), captchaOptions *CaptchaVerifyOptions, log *zap.Logger) http.HandlerFunc {
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
			token := r.Header.Get(captchaTokenHeader)
			if token != "" {
				errorResp, err := createRecaptchaRequest(r.Context(), token, captchaOptions, log)
				if errorResp != nil {
					log.Error(errorResp.message)
					http.Error(w, errorResp.message, errorResp.code)
					return
				} else if err != nil {
					log.Error("site verification failure", zap.Error(err))
					http.Error(w, "site verification failure", http.StatusUnauthorized)
					return
				}
				w.WriteHeader(http.StatusOK)
			} else {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		writeJSON(w, res)
	}
}

// Managing response from reCAPTCHA
func confirm(resp siteVerifyResp, captchaOptions *CaptchaVerifyOptions) error {
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
	if captchaOptions.Threshold != 0 {
		threshold = captchaOptions.Threshold
	}
	if threshold >= *resp.score {
		return fmt.Errorf("received score '%f', while expecting minimum '%f'", *resp.score, threshold)
	}
	return nil
}

// Managing assessment from reCAPTCHA Enterprise
func confirmEnterpriseAssessment(resp *recaptchaenterprisepb.Assessment, captchaOptions *CaptchaVerifyOptions) error {
	if !resp.GetTokenProperties().GetValid() {
		return fmt.Errorf("token is invalid: '%d'", int(resp.GetTokenProperties().GetInvalidReason()))
	}

	threshold := 0.0
	if captchaOptions.Threshold != 0 {
		threshold = captchaOptions.Threshold
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
