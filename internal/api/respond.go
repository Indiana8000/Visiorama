package api

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Code: code, Message: message})
}

func notFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "NOT_FOUND", "resource does not exist")
}

func badRequest(w http.ResponseWriter, message string) {
	writeError(w, http.StatusBadRequest, "INVALID_PARAM", message)
}

func internalError(w http.ResponseWriter) {
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "unexpected server error")
}
