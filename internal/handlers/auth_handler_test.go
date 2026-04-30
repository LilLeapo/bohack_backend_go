package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"bohack_backend_go/internal/auth"
	"bohack_backend_go/internal/config"
	"bohack_backend_go/internal/db"
	"bohack_backend_go/internal/repository"
)

type stubMailer struct {
	mode              string
	verificationSends []verificationSend
}

type verificationSend struct {
	email    string
	code     string
	codeType string
}

type apiResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (m *stubMailer) SendVerificationCode(_ context.Context, email, code, codeType string) error {
	m.verificationSends = append(m.verificationSends, verificationSend{
		email:    email,
		code:     code,
		codeType: codeType,
	})
	return nil
}

func (m *stubMailer) SendAttendanceConfirmation(_ context.Context, _, _, _, _, _ string) error {
	return nil
}

func (m *stubMailer) Mode() string {
	return m.mode
}

func TestSendVerificationCodeStoresCodeWithoutReturningDebugCode(t *testing.T) {
	handler, _, verificationCodes, fakeMailer := newTestAuthHandler(t, true, &stubMailer{mode: "console"})

	resp := performJSONRequest(t, handler.SendVerificationCode, map[string]string{
		"email":     "Alice@Example.com",
		"code_type": "register",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var envelope apiResponse
	decodeJSON(t, resp.Body.Bytes(), &envelope)
	if envelope.Code != 0 {
		t.Fatalf("response code = %d, want 0", envelope.Code)
	}

	var data sendVerificationCodeResponse
	decodeJSON(t, envelope.Data, &data)
	if data.Delivery != "console" {
		t.Fatalf("delivery = %q, want %q", data.Delivery, "console")
	}
	if bytes.Contains(envelope.Data, []byte("debug_code")) {
		t.Fatalf("unexpected debug_code in response data: %s", string(envelope.Data))
	}

	if len(fakeMailer.verificationSends) != 1 {
		t.Fatalf("verification sends = %d, want 1", len(fakeMailer.verificationSends))
	}
	sent := fakeMailer.verificationSends[0]
	if sent.email != "alice@example.com" {
		t.Fatalf("sent email = %q, want %q", sent.email, "alice@example.com")
	}
	if sent.codeType != "register" {
		t.Fatalf("sent code type = %q, want %q", sent.codeType, "register")
	}
	if len(sent.code) != 6 {
		t.Fatalf("sent code length = %d, want 6", len(sent.code))
	}

	saved, err := verificationCodes.GetByEmailAndType(context.Background(), "alice@example.com", "register")
	if err != nil {
		t.Fatalf("load verification code: %v", err)
	}
	if saved.Code != sent.code {
		t.Fatalf("saved code = %q, want %q", saved.Code, sent.code)
	}
}

func TestRegisterSucceedsWithVerificationCodeWhenRequired(t *testing.T) {
	handler, users, verificationCodes, _ := newTestAuthHandler(t, true, &stubMailer{mode: "console"})

	sendResp := performJSONRequest(t, handler.SendVerificationCode, map[string]string{
		"email":     "alice@example.com",
		"code_type": "register",
	})
	if sendResp.Code != http.StatusOK {
		t.Fatalf("send status = %d, want %d, body = %s", sendResp.Code, http.StatusOK, sendResp.Body.String())
	}

	var sendEnvelope apiResponse
	decodeJSON(t, sendResp.Body.Bytes(), &sendEnvelope)

	registerResp := performJSONRequest(t, handler.Register, map[string]string{
		"username":          "alice",
		"email":             "alice@example.com",
		"password":          "secret123",
		"verification_code": fakeVerificationCode(t, handler, "alice@example.com", "register", verificationCodes),
	})
	if registerResp.Code != http.StatusOK {
		t.Fatalf("register status = %d, want %d, body = %s", registerResp.Code, http.StatusOK, registerResp.Body.String())
	}

	exists, err := users.ExistsByEmail(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatalf("check user existence: %v", err)
	}
	if !exists {
		t.Fatal("expected user to be created")
	}

	if _, err := verificationCodes.GetByEmailAndType(context.Background(), "alice@example.com", "register"); err == nil {
		t.Fatal("expected verification code to be deleted after successful registration")
	}
}

func TestRegisterSkipsVerificationWhenDisabled(t *testing.T) {
	handler, users, _, _ := newTestAuthHandler(t, false, &stubMailer{mode: "console"})

	resp := performJSONRequest(t, handler.Register, map[string]string{
		"username": "bob",
		"email":    "bob@example.com",
		"password": "secret123",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	exists, err := users.ExistsByEmail(context.Background(), "bob@example.com")
	if err != nil {
		t.Fatalf("check user existence: %v", err)
	}
	if !exists {
		t.Fatal("expected user to be created")
	}
}

func newTestAuthHandler(t *testing.T, requireRegisterVerification bool, fakeMailer *stubMailer) (*AuthHandler, *repository.UserRepository, *repository.VerificationCodeRepository, *stubMailer) {
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
	verificationRepo := repository.NewVerificationCodeRepository(gormDB)
	handler := NewAuthHandler(
		userRepo,
		auth.NewTokenManager("test-secret", time.Hour),
		verificationRepo,
		fakeMailer,
		requireRegisterVerification,
		10*time.Minute,
		time.Minute,
	)

	return handler, userRepo, verificationRepo, fakeMailer
}

func performJSONRequest(t *testing.T, handler func(http.ResponseWriter, *http.Request), payload any) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	handler(resp, req)
	return resp
}

func decodeJSON(t *testing.T, raw []byte, dst any) {
	t.Helper()

	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}

func fakeVerificationCode(t *testing.T, _ *AuthHandler, email, codeType string, repo *repository.VerificationCodeRepository) string {
	t.Helper()

	item, err := repo.GetByEmailAndType(context.Background(), email, codeType)
	if err != nil {
		t.Fatalf("load verification code: %v", err)
	}
	return item.Code
}
