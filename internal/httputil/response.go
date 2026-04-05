package httputil

import (
	"encoding/json"
	"fmt"
	"net/http"
)

var DefaultCacheMaxAge int

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func JSON(w http.ResponseWriter, status int, data any, cacheSeconds ...int) {
	cacheAge := DefaultCacheMaxAge
	if len(cacheSeconds) > 0 {
		cacheAge = cacheSeconds[0]
	}
	if cacheAge > 0 {
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d, s-maxage=%d", cacheAge, cacheAge))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	}, 0)
}
