package server

import (
	"net/http"
	"time"

	"bohack_backend_go/internal/auth"
	"bohack_backend_go/internal/handlers"
	"bohack_backend_go/internal/httpx"
	"bohack_backend_go/internal/repository"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(
	allowedOrigins []string,
	tokens *auth.TokenManager,
	users *repository.UserRepository,
	authHandler *handlers.AuthHandler,
	profileHandler *handlers.ProfileHandler,
	eventHandler *handlers.EventHandler,
	adminEventHandler *handlers.AdminEventHandler,
	registrationHandler *handlers.RegistrationHandler,
	attachmentHandler *handlers.AttachmentHandler,
	adminRegistrationHandler *handlers.AdminRegistrationHandler,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(httpx.CORSMiddleware(allowedOrigins))

	authMiddleware := httpx.AuthMiddleware(tokens, users)
	adminMiddleware := httpx.AdminMiddleware()

	mountAPIRoutes := func(router chi.Router) {
		router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
			httpx.OK(w, map[string]string{"status": "ok"}, "OK")
		})

		router.Route("/auth", func(r chi.Router) {
			r.Post("/send-verification-code", authHandler.SendVerificationCode)
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
			r.Post("/forgot-password/send-code", authHandler.ForgotPasswordSendCode)
			r.Post("/forgot-password/reset", authHandler.ForgotPasswordReset)
			r.With(authMiddleware).Get("/me", profileHandler.Me)
			r.With(authMiddleware).Post("/change-password", authHandler.ChangePassword)
		})

		router.Get("/events", eventHandler.ListPublic)
		router.Get("/events/current", eventHandler.GetCurrent)
		router.Get("/events/{slug}", eventHandler.GetPublicBySlug)

		router.With(authMiddleware).Get("/user/profile", profileHandler.Me)
		router.With(authMiddleware).Patch("/user/profile", profileHandler.Update)
		router.With(authMiddleware).Post("/registration", registrationHandler.Create)
		router.With(authMiddleware).Put("/registration", registrationHandler.Update)
		router.With(authMiddleware).Patch("/registration", registrationHandler.Update)
		router.With(authMiddleware).Delete("/registration", registrationHandler.Cancel)
		router.With(authMiddleware).Get("/registration/status", registrationHandler.Status)
		router.With(authMiddleware).Get("/registration/attachments", attachmentHandler.ListMy)
		router.With(authMiddleware).Post("/registration/attachments", attachmentHandler.Upload)
		router.With(authMiddleware).Delete("/registration/attachments/{attachmentID}", attachmentHandler.Delete)
		router.With(authMiddleware).Get("/registration/attachments/{attachmentID}/download", attachmentHandler.Download)
		router.With(authMiddleware).Post("/registrations", registrationHandler.Create)
		router.With(authMiddleware).Put("/registrations", registrationHandler.Update)
		router.With(authMiddleware).Patch("/registrations", registrationHandler.Update)
		router.With(authMiddleware).Get("/registrations/me", registrationHandler.Status)
		router.With(authMiddleware).Delete("/registrations/me", registrationHandler.Cancel)
		router.With(authMiddleware).Get("/registrations/me/attachments", attachmentHandler.ListMy)
		router.With(authMiddleware).Post("/registrations/me/attachments", attachmentHandler.Upload)
		router.With(authMiddleware).Delete("/registrations/me/attachments/{attachmentID}", attachmentHandler.Delete)
		router.With(authMiddleware).Get("/registrations/me/attachments/{attachmentID}/download", attachmentHandler.Download)

		router.Route("/admin", func(r chi.Router) {
			r.Use(authMiddleware)
			r.Use(adminMiddleware)
			r.Get("/events", adminEventHandler.List)
			r.Get("/events/{eventID}", adminEventHandler.Detail)
			r.Post("/events", adminEventHandler.Create)
			r.Patch("/events/{eventID}", adminEventHandler.Update)
			r.Get("/registrations", adminRegistrationHandler.List)
			r.Get("/registrations/{registrationID}", adminRegistrationHandler.Detail)
			r.Get("/registrations/{registrationID}/attachments", attachmentHandler.AdminListForRegistration)
			r.Patch("/registrations/{registrationID}/review", adminRegistrationHandler.Review)
		})
	}

	mountAPIRoutes(r)
	r.Route("/api", mountAPIRoutes)

	return r
}
