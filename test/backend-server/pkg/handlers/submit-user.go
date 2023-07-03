package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	chi "github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type errorResp struct {
	Code    int
	Message string
}

type successResp struct {
	Code    int
	Message string
}

type storeReqPayload struct {
	User  string `json:"name"`
	Email string `json:"email"`
}

func SubmitUser(mux chi.Router, log *zap.Logger) {
	mux.Post("/submit", func(w http.ResponseWriter, r *http.Request) {
		var storeRequest storeReqPayload
		errorResp := errorResp{}

		decoder := json.NewDecoder(r.Body)
		decoderErr := decoder.Decode(&storeRequest)
		defer r.Body.Close()

		if decoderErr != nil {
			returnErrorRespAsHttpResponse(w, errorResp)
			return
		} else {
			if storeRequest.User == "" {
				errorResp.Code = http.StatusBadRequest
				errorResp.Message = "Name of the user is required"
				returnErrorRespAsHttpResponse(w, errorResp)
				return
			} else if storeRequest.Email == "" {
				errorResp.Code = http.StatusBadRequest
				errorResp.Message = "User's email address is required"
				returnErrorRespAsHttpResponse(w, errorResp)
				return
			}
			log.Info("Name of user", zap.String("name", storeRequest.User))
			log.Info("Email of user", zap.String("email", storeRequest.Email))

			var successResp = successResp{
				Code:    http.StatusOK,
				Message: "Your request is verified",
			}

			successJSONResp, jsonError := json.Marshal(successResp)
			if jsonError != nil {
				returnErrorMsgAsHttpResponse(w, "Unable to generate a response")
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(successJSONResp)
			return
		}
	})
}

func returnErrorRespAsHttpResponse(r http.ResponseWriter, errorR errorResp) {
	httpResponse := &errorResp{Code: errorR.Code, Message: errorR.Message}
	jsonResponse, err := json.Marshal(httpResponse)
	if err != nil {
		panic(err)
	}
	r.Header().Set("Content-Type", "application/json")
	r.WriteHeader(errorR.Code)
	r.Write(jsonResponse)
}

func returnErrorMsgAsHttpResponse(r http.ResponseWriter, errorMsg string) {
	errorR := errorResp{Code: http.StatusInternalServerError, Message: errorMsg}
	returnErrorRespAsHttpResponse(r, errorR)
}

func getEnvVarOrError(name string) string {
	v, ok := os.LookupEnv(name)
	if !ok {
		panic(errors.New(fmt.Sprintf("given env var is not present, %s not found !", name)))
	}
	return v
}
