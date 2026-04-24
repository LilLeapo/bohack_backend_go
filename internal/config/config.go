package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port                        string
	DBDriver                    string
	DatabaseURL                 string
	JWTSecret                   string
	AccessTokenTTL              time.Duration
	DefaultEventSlug            string
	DefaultEventTitle           string
	AllowedOrigins              []string
	AttachmentDir               string
	MaxUploadBytes              int64
	MailMode                    string
	SMTPHost                    string
	SMTPPort                    int
	SMTPUsername                string
	SMTPPassword                string
	SMTPFrom                    string
	VerificationTTL             time.Duration
	VerificationGap             time.Duration
	RequireRegisterVerification bool
}

func Load() (Config, error) {
	driver := normalizeDriver(envOrDefault("DB_DRIVER", "postgres"))

	cfg := Config{
		Port:              envOrDefault("PORT", "8080"),
		DBDriver:          driver,
		DatabaseURL:       databaseURL(driver),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		DefaultEventSlug:  envOrDefault("DEFAULT_EVENT_SLUG", "bohack-2026"),
		DefaultEventTitle: envOrDefault("DEFAULT_EVENT_TITLE", "BoHack 2026"),
		AllowedOrigins:    parseAllowedOrigins(envOrDefault("ALLOWED_ORIGINS", "*")),
		AttachmentDir:     envOrDefault("ATTACHMENT_DIR", "./storage/registration_attachments"),
		MailMode:          strings.ToLower(envOrDefault("MAIL_MODE", "console")),
		SMTPHost:          strings.TrimSpace(os.Getenv("SMTP_HOST")),
		SMTPUsername:      strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
		SMTPPassword:      os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:          strings.TrimSpace(os.Getenv("SMTP_FROM")),
	}

	if cfg.DatabaseURL == "" {
		switch cfg.DBDriver {
		case "sqlite":
			return Config{}, errors.New("SQLITE_PATH or DATABASE_URL is required when DB_DRIVER=sqlite")
		default:
			return Config{}, errors.New("DATABASE_URL or PG* environment variables are required")
		}
	}
	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}

	ttlMinutes, err := strconv.Atoi(envOrDefault("ACCESS_TOKEN_TTL_MINUTES", "720"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid ACCESS_TOKEN_TTL_MINUTES: %w", err)
	}
	if ttlMinutes <= 0 {
		return Config{}, errors.New("ACCESS_TOKEN_TTL_MINUTES must be greater than 0")
	}
	cfg.AccessTokenTTL = time.Duration(ttlMinutes) * time.Minute

	maxUploadMB, err := strconv.Atoi(envOrDefault("MAX_UPLOAD_MB", "20"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid MAX_UPLOAD_MB: %w", err)
	}
	if maxUploadMB <= 0 {
		return Config{}, errors.New("MAX_UPLOAD_MB must be greater than 0")
	}
	cfg.MaxUploadBytes = int64(maxUploadMB) * 1024 * 1024

	smtpPort, err := strconv.Atoi(envOrDefault("SMTP_PORT", "587"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid SMTP_PORT: %w", err)
	}
	if smtpPort <= 0 {
		return Config{}, errors.New("SMTP_PORT must be greater than 0")
	}
	cfg.SMTPPort = smtpPort

	verifyTTLMinutes, err := strconv.Atoi(envOrDefault("VERIFICATION_CODE_EXPIRE_MINUTES", "10"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid VERIFICATION_CODE_EXPIRE_MINUTES: %w", err)
	}
	if verifyTTLMinutes <= 0 {
		return Config{}, errors.New("VERIFICATION_CODE_EXPIRE_MINUTES must be greater than 0")
	}
	cfg.VerificationTTL = time.Duration(verifyTTLMinutes) * time.Minute

	verifyGapSeconds, err := strconv.Atoi(envOrDefault("VERIFICATION_CODE_MIN_INTERVAL_SECONDS", "60"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid VERIFICATION_CODE_MIN_INTERVAL_SECONDS: %w", err)
	}
	if verifyGapSeconds < 0 {
		return Config{}, errors.New("VERIFICATION_CODE_MIN_INTERVAL_SECONDS must be 0 or greater")
	}
	cfg.VerificationGap = time.Duration(verifyGapSeconds) * time.Second

	requireRegisterVerification, err := strconv.ParseBool(envOrDefault("REQUIRE_REGISTER_VERIFICATION", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid REQUIRE_REGISTER_VERIFICATION: %w", err)
	}
	cfg.RequireRegisterVerification = requireRegisterVerification

	switch cfg.MailMode {
	case "console", "smtp":
	default:
		return Config{}, errors.New("MAIL_MODE must be either console or smtp")
	}

	if cfg.MailMode == "smtp" {
		switch {
		case cfg.SMTPHost == "":
			return Config{}, errors.New("SMTP_HOST is required when MAIL_MODE=smtp")
		case cfg.SMTPFrom == "":
			return Config{}, errors.New("SMTP_FROM is required when MAIL_MODE=smtp")
		}
	}

	return cfg, nil
}

func databaseURL(driver string) string {
	if raw := os.Getenv("DATABASE_URL"); raw != "" {
		return raw
	}

	if driver == "sqlite" {
		path := strings.TrimSpace(os.Getenv("SQLITE_PATH"))
		if path == "" {
			return ""
		}
		return path
	}

	host := os.Getenv("PGHOST")
	port := envOrDefault("PGPORT", "5432")
	dbname := os.Getenv("PGDATABASE")
	user := os.Getenv("PGUSER")
	password := os.Getenv("PGPASSWORD")

	if host == "" || dbname == "" || user == "" {
		return ""
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%s", host, port),
		Path:   dbname,
	}

	q := u.Query()
	q.Set("sslmode", envOrDefault("PGSSLMODE", "disable"))
	u.RawQuery = q.Encode()

	return u.String()
}

func normalizeDriver(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "sqlite", "sqlite3":
		return "sqlite"
	case "", "postgres", "postgresql", "pg":
		return "postgres"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func parseAllowedOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, item := range parts {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return []string{"*"}
	}
	return out
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
