package httpapi

import (
	"encoding/json"
	"net/http"
)

type apiResponse struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func writeOK(w http.ResponseWriter, r *http.Request, data any) {
	writeJSON(w, r, http.StatusOK, http.StatusOK, "ok", data)
}

func writeCreated(w http.ResponseWriter, r *http.Request, data any) {
	writeJSON(w, r, http.StatusCreated, http.StatusCreated, "created", data)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, message string) {
	writeJSON(w, r, status, status, message, nil)
}

func writeErrorData(w http.ResponseWriter, r *http.Request, status int, message string, data any) {
	writeJSON(w, r, status, status, message, data)
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, code int, message string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiResponse{
		Code:      code,
		Message:   message,
		Data:      data,
		RequestID: requestIDFromContext(r.Context()),
	})
}
