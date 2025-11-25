package middleware

import (
	"context"
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

type ctxKey int

const RequestIDKey ctxKey = iota

// RequestID is a middleware that propagates chi's request ID to our context key.
// It must be used AFTER chi's RequestID middleware in the chain.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := chimw.GetReqID(r.Context())
		ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
		w.Header().Set("X-Request-Id", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID retrieves the request ID from context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}
