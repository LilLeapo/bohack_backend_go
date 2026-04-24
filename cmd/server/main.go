package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"bohack_backend_go/internal/auth"
	"bohack_backend_go/internal/config"
	"bohack_backend_go/internal/db"
	"bohack_backend_go/internal/handlers"
	"bohack_backend_go/internal/mailer"
	"bohack_backend_go/internal/repository"
	"bohack_backend_go/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	cfg.AttachmentDir, err = filepath.Abs(cfg.AttachmentDir)
	if err != nil {
		log.Fatalf("resolve attachment dir: %v", err)
	}
	if err := os.MkdirAll(cfg.AttachmentDir, 0o755); err != nil {
		log.Fatalf("create attachment dir: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	gormDB, err := db.Open(ctx, cfg)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() {
		if err := db.Close(gormDB); err != nil {
			log.Printf("close database: %v", err)
		}
	}()

	if err := db.EnsureSchema(ctx, gormDB, cfg); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}

	userRepo := repository.NewUserRepository(gormDB)
	eventRepo := repository.NewEventRepository(gormDB)
	registrationRepo := repository.NewRegistrationRepository(gormDB)
	attachmentRepo := repository.NewAttachmentRepository(gormDB)
	verificationCodeRepo := repository.NewVerificationCodeRepository(gormDB)

	tokenManager := auth.NewTokenManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	var authMailer mailer.Mailer
	switch cfg.MailMode {
	case "smtp":
		authMailer = mailer.NewSMTPMailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom)
	default:
		authMailer = mailer.NewConsoleMailer()
	}

	authHandler := handlers.NewAuthHandler(
		userRepo,
		tokenManager,
		verificationCodeRepo,
		authMailer,
		cfg.RequireRegisterVerification,
		cfg.VerificationTTL,
		cfg.VerificationGap,
	)
	profileHandler := handlers.NewProfileHandler(userRepo)
	eventHandler := handlers.NewEventHandler(eventRepo, cfg.DefaultEventSlug)
	adminEventHandler := handlers.NewAdminEventHandler(eventRepo)
	registrationHandler := handlers.NewRegistrationHandler(eventRepo, registrationRepo, cfg.DefaultEventSlug)
	attachmentHandler := handlers.NewAttachmentHandler(
		eventRepo,
		registrationRepo,
		attachmentRepo,
		cfg.DefaultEventSlug,
		cfg.AttachmentDir,
		cfg.MaxUploadBytes,
	)
	adminRegistrationHandler := handlers.NewAdminRegistrationHandler(eventRepo, registrationRepo)

	router := server.NewRouter(
		cfg.AllowedOrigins,
		tokenManager,
		userRepo,
		authHandler,
		profileHandler,
		eventHandler,
		adminEventHandler,
		registrationHandler,
		attachmentHandler,
		adminRegistrationHandler,
	)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("bohack backend listening on :%s", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen and serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown: %v", err)
	}
}
