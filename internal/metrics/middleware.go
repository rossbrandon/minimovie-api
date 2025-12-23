package metrics

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if M == nil {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		wrapped := newResponseWriter(w)
		next.ServeHTTP(wrapped, r)

		ctx := r.Context()
		routePattern := chi.RouteContext(ctx).RoutePattern()
		if routePattern == "" {
			routePattern = "unknown"
		}

		M.RecordHttpRequest(ctx, r.Method, routePattern, wrapped.statusCode, time.Since(start))
	})
}
