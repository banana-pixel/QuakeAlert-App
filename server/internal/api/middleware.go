package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ctxKey adalah tipe privat untuk key context (menghindari tabrakan).
type ctxKey int

const userIDKey ctxKey = iota

// UserIDFromContext mengambil user_id (sub JWT) yang di-set oleh AuthMiddleware.
func UserIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userIDKey).(string)
	return v, ok
}

// AuthMiddleware memvalidasi header Authorization: Bearer <jwt> (HS256) dan
// menaruh user_id ke context. Tanpa/invalid token → 401 (life-safety: endpoint
// tidak boleh terbuka tanpa auth di produksi).
func AuthMiddleware(secret []byte, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(authz, prefix) {
				writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "header Authorization Bearer diperlukan")
				return
			}
			token := strings.TrimSpace(authz[len(prefix):])
			claims, err := verifyHS256(token, secret, time.Now())
			if err != nil {
				log.Debug("jwt tidak valid", "err", err)
				writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "token tidak valid")
				return
			}
			ctx := context.WithValue(r.Context(), userIDKey, claims.Sub)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
