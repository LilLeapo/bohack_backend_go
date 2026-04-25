package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bohack_backend_go/internal/httpx"
	"bohack_backend_go/internal/mailer"
	"bohack_backend_go/internal/repository"
)

type AttendanceHandler struct {
	registrations   *repository.RegistrationRepository
	confirmations   *repository.AttendanceConfirmationRepository
	mailer          mailer.Mailer
	frontendBaseURL string
	ttl             time.Duration
}

type attendanceConfirmRequest struct {
	Token  string `json:"token"`
	Status string `json:"status"`
}

func NewAttendanceHandler(
	registrations *repository.RegistrationRepository,
	confirmations *repository.AttendanceConfirmationRepository,
	mailer mailer.Mailer,
	frontendBaseURL string,
	ttl time.Duration,
) *AttendanceHandler {
	return &AttendanceHandler{
		registrations:   registrations,
		confirmations:   confirmations,
		mailer:          mailer,
		frontendBaseURL: strings.TrimRight(frontendBaseURL, "/"),
		ttl:             ttl,
	}
}

func (h *AttendanceHandler) AdminSend(w http.ResponseWriter, r *http.Request) {
	registrationID, ok := readRegistrationID(w, r)
	if !ok {
		return
	}

	registration, err := h.registrations.GetByID(r.Context(), registrationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40411, "registration not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50032, "failed to load registration")
		return
	}

	token, tokenHash, err := generateAttendanceToken()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50080, "failed to generate confirmation token")
		return
	}

	expiresAt := time.Now().UTC().Add(h.ttl)
	confirmation, err := h.confirmations.Create(r.Context(), repository.CreateAttendanceConfirmationParams{
		RegistrationID: registration.ID,
		UserID:         registration.UserID,
		TokenHash:      tokenHash,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50081, "failed to create attendance confirmation")
		return
	}

	confirmURL := h.attendanceURL(token, "confirmed")
	declineURL := h.attendanceURL(token, "declined")
	if err := h.mailer.SendAttendanceConfirmation(
		r.Context(),
		registration.EmailSnapshot,
		registration.RealName,
		registration.EventTitle,
		confirmURL,
		declineURL,
	); err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50082, "failed to send attendance confirmation email")
		return
	}

	httpx.OK(w, map[string]any{
		"confirmation": confirmation,
		"delivery":     h.mailer.Mode(),
		"confirmUrl":   consoleOnlyURL(h.mailer.Mode(), confirmURL),
		"declineUrl":   consoleOnlyURL(h.mailer.Mode(), declineURL),
	}, "attendance confirmation sent")
}

func (h *AttendanceHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	req := attendanceConfirmRequest{
		Token:  strings.TrimSpace(r.URL.Query().Get("token")),
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
	}
	if r.Method != http.MethodGet {
		var body attendanceConfirmRequest
		if !httpx.DecodeJSON(w, r, &body) {
			return
		}
		req.Token = firstNonEmpty(body.Token, req.Token)
		req.Status = firstNonEmpty(body.Status, req.Status)
	}

	req.Status = strings.TrimSpace(strings.ToLower(req.Status))
	if req.Token == "" {
		httpx.Error(w, http.StatusBadRequest, 42280, "token is required")
		return
	}
	if !isAllowedAttendanceStatus(req.Status) {
		httpx.Error(w, http.StatusBadRequest, 42281, "status must be confirmed or declined")
		return
	}

	tokenHash := hashAttendanceToken(req.Token)
	confirmation, err := h.confirmations.GetByTokenHash(r.Context(), tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40480, "attendance confirmation not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50083, "failed to load attendance confirmation")
		return
	}
	if time.Now().UTC().After(confirmation.ExpiresAt) {
		httpx.Error(w, http.StatusGone, 41080, "attendance confirmation link has expired")
		return
	}

	updated, err := h.confirmations.Respond(r.Context(), tokenHash, req.Status)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50084, "failed to update attendance confirmation")
		return
	}

	registration, err := h.registrations.GetByID(r.Context(), updated.RegistrationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40411, "registration not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50032, "failed to load registration")
		return
	}
	registration.Attendance = updated

	httpx.OK(w, map[string]any{
		"attendance":   updated,
		"registration": registration,
	}, "attendance confirmed")
}

func (h *AttendanceHandler) attendanceURL(token, status string) string {
	u, err := url.Parse(h.frontendBaseURL + "/attendance-confirm")
	if err != nil {
		return h.frontendBaseURL + "/attendance-confirm?token=" + url.QueryEscape(token) + "&status=" + url.QueryEscape(status)
	}
	q := u.Query()
	q.Set("token", token)
	q.Set("status", status)
	u.RawQuery = q.Encode()
	return u.String()
}

func generateAttendanceToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(raw)
	return token, hashAttendanceToken(token), nil
}

func hashAttendanceToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func isAllowedAttendanceStatus(status string) bool {
	switch status {
	case "confirmed", "declined":
		return true
	default:
		return false
	}
}

func consoleOnlyURL(mode, value string) string {
	if mode == "console" {
		return value
	}
	return ""
}
