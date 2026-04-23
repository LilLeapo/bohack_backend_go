package httpx

import (
	"encoding/json"
	"io"
	"net/http"
)

type envelope struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func OK(w http.ResponseWriter, data interface{}, message string) {
	if message == "" {
		message = "OK"
	}
	writeJSON(w, http.StatusOK, envelope{
		Code:    0,
		Message: message,
		Data:    data,
	})
}

func Error(w http.ResponseWriter, status int, code int, message string) {
	writeJSON(w, status, envelope{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}

func DecodeJSON(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		Error(w, http.StatusBadRequest, 40000, "invalid request body")
		return false
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		Error(w, http.StatusBadRequest, 40000, "request body must contain a single JSON object")
		return false
	}

	return true
}

func writeJSON(w http.ResponseWriter, status int, payload envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
