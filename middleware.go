package txflow

import (
	"net/http"

	"gorm.io/gorm"
)

// TxManagerHttpMiddleware attaches the provided root *gorm.DB to each incoming request's context.
func TxManagerHttpMiddleware(db *gorm.DB) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if db == nil {
				next.ServeHTTP(w, r)
			} else {
				next.ServeHTTP(w, r.WithContext(WithDB(ctx, db)))
			}
		})
	}
}
