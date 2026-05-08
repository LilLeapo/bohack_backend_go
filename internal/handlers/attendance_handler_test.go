package handlers

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"bohack_backend_go/internal/config"
	"bohack_backend_go/internal/db"
	"bohack_backend_go/internal/models"
	"bohack_backend_go/internal/repository"
)

func TestConfirmUploadStoresSignedFileAndConfirmsAttendance(t *testing.T) {
	handler, confirmationRepo, attachmentRepo, token, registrationID := newTestAttendanceHandler(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("token", token); err != nil {
		t.Fatalf("write token field: %v", err)
	}
	part, err := writer.CreateFormFile("file", "signed-confirmation.txt")
	if err != nil {
		t.Fatalf("create file field: %v", err)
	}
	if _, err := part.Write([]byte("signed confirmation")); err != nil {
		t.Fatalf("write file field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/attendance/confirm/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp := httptest.NewRecorder()
	handler.ConfirmUpload(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	updated, err := confirmationRepo.GetByTokenHash(context.Background(), hashAttendanceToken(token))
	if err != nil {
		t.Fatalf("load attendance confirmation: %v", err)
	}
	if updated.Status != "confirmed" {
		t.Fatalf("attendance status = %q, want confirmed", updated.Status)
	}

	attachments, err := attachmentRepo.ListByRegistration(context.Background(), registrationID)
	if err != nil {
		t.Fatalf("list attachments: %v", err)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments len = %d, want 1", len(attachments))
	}
	if attachments[0].Kind != "risk_confirmation" {
		t.Fatalf("attachment kind = %q, want risk_confirmation", attachments[0].Kind)
	}
}

func TestConfirmRejectsConfirmedWithoutSignedFile(t *testing.T) {
	handler, _, _, token, _ := newTestAttendanceHandler(t)

	resp := performJSONRequest(t, handler.Confirm, map[string]string{
		"token":  token,
		"status": "confirmed",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
}

func newTestAttendanceHandler(t *testing.T) (*AttendanceHandler, *repository.AttendanceConfirmationRepository, *repository.AttachmentRepository, string, int64) {
	t.Helper()

	cfg := config.Config{
		DBDriver:          "sqlite",
		DatabaseURL:       filepath.Join(t.TempDir(), "test.db"),
		DefaultEventSlug:  "test-event",
		DefaultEventTitle: "Test Event",
	}

	ctx := context.Background()
	gormDB, err := db.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(gormDB); err != nil {
			t.Fatalf("close test database: %v", err)
		}
	})

	if err := db.EnsureSchema(ctx, gormDB, cfg); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	userRepo := repository.NewUserRepository(gormDB)
	eventRepo := repository.NewEventRepository(gormDB)
	registrationRepo := repository.NewRegistrationRepository(gormDB)
	confirmationRepo := repository.NewAttendanceConfirmationRepository(gormDB)
	attachmentRepo := repository.NewAttachmentRepository(gormDB)

	now := time.Now().UTC()
	user := models.User{
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "hash",
		Role:         "visitor",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := userRepo.Create(ctx, &user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	event, err := eventRepo.GetBySlug(ctx, cfg.DefaultEventSlug)
	if err != nil {
		t.Fatalf("load event: %v", err)
	}
	registration, err := registrationRepo.Create(ctx, repository.CreateRegistrationParams{
		EventID:       event.ID,
		UserID:        user.UID,
		Status:        "approved",
		RealName:      "Alice",
		Phone:         "13800138000",
		EmailSnapshot: user.Email,
	})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	token, tokenHash, err := generateAttendanceToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := confirmationRepo.Create(ctx, repository.CreateAttendanceConfirmationParams{
		RegistrationID: registration.ID,
		UserID:         user.UID,
		TokenHash:      tokenHash,
		ExpiresAt:      now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("create attendance confirmation: %v", err)
	}

	handler := NewAttendanceHandler(
		registrationRepo,
		confirmationRepo,
		attachmentRepo,
		&stubMailer{mode: "console"},
		"https://bohack.top",
		time.Hour,
		t.TempDir(),
		1024*1024,
	)

	return handler, confirmationRepo, attachmentRepo, token, registration.ID
}
