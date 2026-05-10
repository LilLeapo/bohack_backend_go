package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bohack_backend_go/internal/httpx"
	"bohack_backend_go/internal/mailer"
	"bohack_backend_go/internal/repository"

	"github.com/go-chi/chi/v5"
)

type RegistrationEmailHandler struct {
	registrations   *repository.RegistrationRepository
	confirmations   *repository.AttendanceConfirmationRepository
	mailer          mailer.Mailer
	frontendBaseURL string
	ttl             time.Duration
}

type adminSendRegistrationEmailRequest struct {
	Type            string `json:"type"`
	EmailType       string `json:"email_type"`
	EmailTypeCamel  string `json:"emailType"`
	ConfirmURL      string `json:"confirm_url"`
	ConfirmURLCamel string `json:"confirmUrl"`
}

func NewRegistrationEmailHandler(
	registrations *repository.RegistrationRepository,
	confirmations *repository.AttendanceConfirmationRepository,
	mailer mailer.Mailer,
	frontendBaseURL string,
	ttl time.Duration,
) *RegistrationEmailHandler {
	return &RegistrationEmailHandler{
		registrations:   registrations,
		confirmations:   confirmations,
		mailer:          mailer,
		frontendBaseURL: strings.TrimRight(frontendBaseURL, "/"),
		ttl:             ttl,
	}
}

func (h *RegistrationEmailHandler) AdminSend(w http.ResponseWriter, r *http.Request) {
	registrationID, ok := readRegistrationID(w, r)
	if !ok {
		return
	}

	req := adminSendRegistrationEmailRequest{}
	if r.Body != nil && r.ContentLength != 0 {
		if !httpx.DecodeJSON(w, r, &req) {
			return
		}
	}

	rawKind := firstNonEmpty(
		chi.URLParam(r, "emailType"),
		req.Type,
		req.EmailType,
		req.EmailTypeCamel,
	)
	kind, ok := mailer.ParseRegistrationEmailKind(rawKind)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, 42290, "email type must be admission, visitor, minor_admission, or agreement_reminder")
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

	var confirmation any
	confirmURL := strings.TrimSpace(firstNonEmpty(req.ConfirmURL, req.ConfirmURLCamel))
	if kind.RequiresConfirmURL() && confirmURL == "" {
		token, tokenHash, err := generateAttendanceToken()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, 50080, "failed to generate confirmation token")
			return
		}
		item, err := h.confirmations.Create(r.Context(), repository.CreateAttendanceConfirmationParams{
			RegistrationID: registration.ID,
			UserID:         registration.UserID,
			TokenHash:      tokenHash,
			ExpiresAt:      time.Now().UTC().Add(h.ttl),
		})
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, 50081, "failed to create attendance confirmation")
			return
		}
		confirmation = item
		confirmURL = h.attendanceURL(token, "confirmed")
	}

	if err := h.mailer.SendRegistrationEmail(
		r.Context(),
		registration.EmailSnapshot,
		mailer.RegistrationEmailParams{
			Kind:       kind,
			Name:       registration.RealName,
			ConfirmURL: confirmURL,
		},
	); err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50090, "failed to send registration email")
		return
	}

	httpx.OK(w, map[string]any{
		"confirmation": confirmation,
		"delivery":     h.mailer.Mode(),
		"emailType":    string(kind),
		"confirmUrl":   confirmURL,
	}, "registration email sent")
}

func (h *RegistrationEmailHandler) attendanceURL(token, status string) string {
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
