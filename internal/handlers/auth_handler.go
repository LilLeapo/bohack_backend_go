package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"math/big"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"bohack_backend_go/internal/auth"
	"bohack_backend_go/internal/httpx"
	"bohack_backend_go/internal/mailer"
	"bohack_backend_go/internal/models"
	"bohack_backend_go/internal/repository"

	"github.com/jackc/pgx/v5/pgconn"
)

type AuthHandler struct {
	users                       *repository.UserRepository
	tokens                      *auth.TokenManager
	verificationCodes           *repository.VerificationCodeRepository
	mailer                      mailer.Mailer
	requireRegisterVerification bool
	verificationTTL             time.Duration
	verificationGap             time.Duration
}

type registerRequest struct {
	Username          string `json:"username"`
	Email             string `json:"email"`
	Password          string `json:"password"`
	VerificationCode  string `json:"verification_code"`
	VerificationCode2 string `json:"verificationCode"`
}

type loginRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Login    string `json:"login"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword  string `json:"current_password"`
	CurrentPassword2 string `json:"currentPassword"`
	NewPassword      string `json:"new_password"`
	NewPassword2     string `json:"newPassword"`
}

type sendVerificationCodeRequest struct {
	Email     string `json:"email"`
	CodeType  string `json:"code_type"`
	CodeType2 string `json:"codeType"`
	Purpose   string `json:"purpose"`
	Type      string `json:"type"`
}

type resetPasswordRequest struct {
	Email             string `json:"email"`
	VerificationCode  string `json:"verification_code"`
	VerificationCode2 string `json:"verificationCode"`
	Code              string `json:"code"`
	NewPassword       string `json:"new_password"`
	NewPassword2      string `json:"newPassword"`
}

type authResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	ExpiresAt   time.Time    `json:"expires_at"`
	User        *models.User `json:"user"`
}

type sendVerificationCodeResponse struct {
	ExpireIn  int64  `json:"expire_in"`
	Delivery  string `json:"delivery"`
	DebugCode string `json:"debug_code,omitempty"`
}

func NewAuthHandler(
	users *repository.UserRepository,
	tokens *auth.TokenManager,
	verificationCodes *repository.VerificationCodeRepository,
	mailer mailer.Mailer,
	requireRegisterVerification bool,
	verificationTTL time.Duration,
	verificationGap time.Duration,
) *AuthHandler {
	return &AuthHandler{
		users:                       users,
		tokens:                      tokens,
		verificationCodes:           verificationCodes,
		mailer:                      mailer,
		requireRegisterVerification: requireRegisterVerification,
		verificationTTL:             verificationTTL,
		verificationGap:             verificationGap,
	}
}

func (h *AuthHandler) SendVerificationCode(w http.ResponseWriter, r *http.Request) {
	h.handleSendVerificationCode(w, r, "")
}

func (h *AuthHandler) ForgotPasswordSendCode(w http.ResponseWriter, r *http.Request) {
	h.handleSendVerificationCode(w, r, "reset")
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.VerificationCode = firstNonEmpty(req.VerificationCode, req.VerificationCode2)

	switch {
	case req.Username == "" || tooLong(req.Username, 50):
		httpx.Error(w, http.StatusBadRequest, 42201, "username must be between 1 and 50 characters")
		return
	case req.Email == "":
		httpx.Error(w, http.StatusBadRequest, 42202, "email is required")
		return
	case len(req.Password) < 6 || len(req.Password) > 128:
		httpx.Error(w, http.StatusBadRequest, 42203, "password must be between 6 and 128 characters")
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		httpx.Error(w, http.StatusBadRequest, 42206, "invalid email address")
		return
	}

	if h.requireRegisterVerification {
		if len(req.VerificationCode) != 6 {
			httpx.Error(w, http.StatusBadRequest, 42212, "verification_code must be 6 digits")
			return
		}
		if _, err := h.validateVerificationCode(r.Context(), req.Email, "register", req.VerificationCode); err != nil {
			httpx.Error(w, http.StatusBadRequest, 40006, "verification code is invalid or expired")
			return
		}
	}

	exists, err := h.users.ExistsByUsername(r.Context(), req.Username)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50001, "failed to check username")
		return
	}
	if exists {
		httpx.Error(w, http.StatusConflict, 40901, "username already exists")
		return
	}

	exists, err = h.users.ExistsByEmail(r.Context(), req.Email)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50002, "failed to check email")
		return
	}
	if exists {
		httpx.Error(w, http.StatusConflict, 40902, "email already exists")
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50003, "failed to hash password")
		return
	}

	uid, err := h.users.GenerateUID(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50004, "failed to generate user id")
		return
	}

	now := time.Now().UTC()
	user := &models.User{
		UID:          uid,
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: passwordHash,
		IsAdmin:      false,
		Role:         "visitor",
		BKBalance:    0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := h.users.Create(r.Context(), user); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			httpx.Error(w, http.StatusConflict, 40903, "username or email already exists")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50005, "failed to create user")
		return
	}

	if h.requireRegisterVerification {
		_ = h.verificationCodes.DeleteByEmailAndType(r.Context(), req.Email, "register")
	}

	h.respondWithToken(w, user, "registration successful")
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Login = strings.TrimSpace(req.Login)

	if req.Username == "" && req.Email == "" && req.Login != "" {
		if strings.Contains(req.Login, "@") {
			req.Email = strings.ToLower(req.Login)
		} else {
			req.Username = req.Login
		}
	}

	if req.Username == "" && req.Email == "" {
		httpx.Error(w, http.StatusBadRequest, 42204, "username or email is required")
		return
	}
	if len(req.Password) < 6 || len(req.Password) > 128 {
		httpx.Error(w, http.StatusBadRequest, 42205, "password must be between 6 and 128 characters")
		return
	}

	user, err := h.users.GetByLogin(r.Context(), req.Username, req.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusUnauthorized, 40103, "invalid credentials")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50006, "failed to load user")
		return
	}

	if err := auth.ComparePassword(user.PasswordHash, req.Password); err != nil {
		httpx.Error(w, http.StatusUnauthorized, 40104, "invalid credentials")
		return
	}

	h.respondWithToken(w, user, "login successful")
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user := httpx.CurrentUser(r)
	if user == nil {
		httpx.Error(w, http.StatusUnauthorized, 40113, "unauthorized")
		return
	}

	var req changePasswordRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	req.CurrentPassword = firstNonEmpty(req.CurrentPassword, req.CurrentPassword2)
	req.NewPassword = firstNonEmpty(req.NewPassword, req.NewPassword2)

	switch {
	case len(req.CurrentPassword) < 6 || len(req.CurrentPassword) > 128:
		httpx.Error(w, http.StatusBadRequest, 42207, "current_password must be between 6 and 128 characters")
		return
	case len(req.NewPassword) < 6 || len(req.NewPassword) > 128:
		httpx.Error(w, http.StatusBadRequest, 42208, "new_password must be between 6 and 128 characters")
		return
	case req.CurrentPassword == req.NewPassword:
		httpx.Error(w, http.StatusBadRequest, 42209, "new_password must be different from current_password")
		return
	}

	freshUser, err := h.users.GetByID(r.Context(), user.UID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusUnauthorized, 40102, "user not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50008, "failed to load user")
		return
	}

	if err := auth.ComparePassword(freshUser.PasswordHash, req.CurrentPassword); err != nil {
		httpx.Error(w, http.StatusUnauthorized, 40114, "current password is incorrect")
		return
	}

	newPasswordHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50009, "failed to hash password")
		return
	}

	if err := h.users.UpdatePassword(r.Context(), user.UID, newPasswordHash); err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50010, "failed to update password")
		return
	}

	httpx.OK(w, map[string]string{"status": "updated"}, "password updated")
}

func (h *AuthHandler) ForgotPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.VerificationCode = firstNonEmpty(req.VerificationCode, req.VerificationCode2, req.Code)
	req.NewPassword = firstNonEmpty(req.NewPassword, req.NewPassword2)

	switch {
	case req.Email == "":
		httpx.Error(w, http.StatusBadRequest, 42202, "email is required")
		return
	case len(req.VerificationCode) != 6:
		httpx.Error(w, http.StatusBadRequest, 42212, "verification_code must be 6 digits")
		return
	case len(req.NewPassword) < 6 || len(req.NewPassword) > 128:
		httpx.Error(w, http.StatusBadRequest, 42208, "new_password must be between 6 and 128 characters")
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		httpx.Error(w, http.StatusBadRequest, 42206, "invalid email address")
		return
	}

	user, err := h.users.GetByLogin(r.Context(), "", req.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusBadRequest, 40007, "verification code is invalid or expired")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50011, "failed to load user")
		return
	}

	if _, err := h.validateVerificationCode(r.Context(), req.Email, "reset", req.VerificationCode); err != nil {
		httpx.Error(w, http.StatusBadRequest, 40007, "verification code is invalid or expired")
		return
	}

	newPasswordHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50009, "failed to hash password")
		return
	}

	if err := h.users.UpdatePassword(r.Context(), user.UID, newPasswordHash); err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50010, "failed to update password")
		return
	}

	_ = h.verificationCodes.DeleteByEmailAndType(r.Context(), req.Email, "reset")

	httpx.OK(w, map[string]string{"status": "updated"}, "password reset successful")
}

func (h *AuthHandler) handleSendVerificationCode(w http.ResponseWriter, r *http.Request, forcedCodeType string) {
	var req sendVerificationCodeRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	codeType := normalizeVerificationCodeType(firstNonEmpty(forcedCodeType, req.CodeType, req.CodeType2, req.Purpose, req.Type))

	if req.Email == "" {
		httpx.Error(w, http.StatusBadRequest, 42202, "email is required")
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		httpx.Error(w, http.StatusBadRequest, 42206, "invalid email address")
		return
	}
	if codeType == "" {
		httpx.Error(w, http.StatusBadRequest, 42213, "code_type must be register or reset")
		return
	}

	now := time.Now().UTC()
	shouldIssueCode := true

	switch codeType {
	case "register":
		exists, err := h.users.ExistsByEmail(r.Context(), req.Email)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, 50012, "failed to check email")
			return
		}
		if exists {
			httpx.Error(w, http.StatusConflict, 40902, "email already exists")
			return
		}
	case "reset":
		exists, err := h.users.ExistsByEmail(r.Context(), req.Email)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, 50012, "failed to check email")
			return
		}
		if !exists {
			shouldIssueCode = false
		}
	}

	if shouldIssueCode {
		existingCode, err := h.verificationCodes.GetByEmailAndType(r.Context(), req.Email, codeType)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusInternalServerError, 50013, "failed to load verification code")
			return
		}
		if err == nil && h.verificationGap > 0 && now.Sub(existingCode.LastSendAt) < h.verificationGap {
			httpx.Error(w, http.StatusTooManyRequests, 42901, "please wait before requesting another code")
			return
		}

		code, err := generateVerificationCode()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, 50014, "failed to generate verification code")
			return
		}

		expiresAt := now.Add(h.verificationTTL)
		if err := h.verificationCodes.Upsert(r.Context(), req.Email, codeType, code, expiresAt, now); err != nil {
			httpx.Error(w, http.StatusInternalServerError, 50015, "failed to save verification code")
			return
		}

		if err := h.mailer.SendVerificationCode(r.Context(), req.Email, code, codeType); err != nil {
			httpx.Error(w, http.StatusInternalServerError, 50016, "failed to send verification code")
			return
		}

		response := sendVerificationCodeResponse{
			ExpireIn: int64(h.verificationTTL.Seconds()),
			Delivery: h.mailer.Mode(),
		}
		if h.mailer.Mode() == "console" {
			response.DebugCode = code
		}
		httpx.OK(w, response, "verification code sent")
		return
	}

	httpx.OK(w, sendVerificationCodeResponse{
		ExpireIn: int64(h.verificationTTL.Seconds()),
		Delivery: "accepted",
	}, "verification code sent")
}

func (h *AuthHandler) validateVerificationCode(ctx context.Context, email, codeType, code string) (*models.VerificationCode, error) {
	item, err := h.verificationCodes.GetByEmailAndType(ctx, email, codeType)
	if err != nil {
		return nil, err
	}

	if item.Code != strings.TrimSpace(code) {
		return nil, errors.New("invalid verification code")
	}
	if time.Now().UTC().After(item.ExpiresAt) {
		_ = h.verificationCodes.DeleteByEmailAndType(ctx, email, codeType)
		return nil, errors.New("expired verification code")
	}

	return item, nil
}

func (h *AuthHandler) respondWithToken(w http.ResponseWriter, user *models.User, message string) {
	token, expiresAt, err := h.tokens.CreateAccessToken(user.UID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50007, "failed to create access token")
		return
	}

	httpx.OK(w, authResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt,
		User:        user,
	}, message)
}

func normalizeVerificationCodeType(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", "register", "signup", "registration":
		return "register"
	case "reset", "forgot_password", "forgot-password", "password_reset", "password-reset":
		return "reset"
	default:
		return ""
	}
}

func generateVerificationCode() (string, error) {
	var builder strings.Builder
	builder.Grow(6)

	for i := 0; i < 6; i++ {
		value, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		builder.WriteByte(byte('0' + value.Int64()))
	}

	return builder.String(), nil
}
