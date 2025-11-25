package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					requestID := GetRequestID(r.Context())
					stack := debug.Stack()

					logger.Error("panic recovered",
						slog.String("request_id", requestID),
						slog.Any("panic", rec),
						slog.String("stack", string(stack)),
					)

					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
