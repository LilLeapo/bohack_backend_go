package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"bohack_backend_go/internal/httpx"
	"bohack_backend_go/internal/mailer"
	"bohack_backend_go/internal/repository"

	"github.com/go-chi/chi/v5"
)

type RegistrationEmailHandler struct {
	registrations *repository.RegistrationRepository
	mailer        mailer.Mailer
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
	mailer mailer.Mailer,
) *RegistrationEmailHandler {
	return &RegistrationEmailHandler{
		registrations: registrations,
		mailer:        mailer,
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

	confirmURL := strings.TrimSpace(firstNonEmpty(req.ConfirmURL, req.ConfirmURLCamel))
	if kind.RequiresConfirmURL() && confirmURL == "" {
		httpx.Error(w, http.StatusBadRequest, 42291, "confirm_url is required for admission email")
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
		"delivery":   h.mailer.Mode(),
		"emailType":  string(kind),
		"confirmUrl": consoleOnlyURL(h.mailer.Mode(), confirmURL),
	}, "registration email sent")
}
