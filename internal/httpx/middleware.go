package httpx

import (
	"context"
	"net/http"
	"strings"

	"bohack_backend_go/internal/auth"
	"bohack_backend_go/internal/models"
	"bohack_backend_go/internal/repository"
)

type contextKey string

const userContextKey contextKey = "currentUser"

func CurrentUser(r *http.Request) *models.User {
	user, _ := r.Context().Value(userContextKey).(*models.User)
	return user
}

func AuthMiddleware(tokens *auth.TokenManager, users *repository.UserRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				Error(w, http.StatusUnauthorized, 40100, "missing or invalid authorization header")
				return
			}

			userID, err := tokens.ParseAccessToken(token)
			if err != nil {
				Error(w, http.StatusUnauthorized, 40101, "invalid or expired token")
				return
			}

			user, err := users.GetByID(r.Context(), userID)
			if err != nil {
				Error(w, http.StatusUnauthorized, 40102, "user not found")
				return
			}

			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AdminMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := CurrentUser(r)
			if user == nil {
				Error(w, http.StatusUnauthorized, 40108, "unauthorized")
				return
			}
			if !user.IsAdmin && !strings.EqualFold(user.Role, "admin") {
				Error(w, http.StatusForbidden, 40301, "admin access required")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := len(allowedOrigins) == 1 && allowedOrigins[0] == "*"
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if allowAll && origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin != "" {
				if _, ok := allowed[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
				}
			}

			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	return parts[1], true
}
