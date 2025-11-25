package handler

import (
	"encoding/json"
	"net/http"
)

func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
	}
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func Error(w http.ResponseWriter, status int, err string, message string) {
	JSON(w, status, ErrorResponse{
		Error:   err,
		Message: message,
	})
}
