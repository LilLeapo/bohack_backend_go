package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"bohack_backend_go/internal/httpx"
	"bohack_backend_go/internal/models"
	"bohack_backend_go/internal/repository"

	"github.com/jackc/pgx/v5/pgconn"
)

type RegistrationHandler struct {
	events        *repository.EventRepository
	registrations *repository.RegistrationRepository
	defaultSlug   string
}

type registrationPayload struct {
	EventSlug           string         `json:"event_slug"`
	EventSlugCamel      string         `json:"eventSlug"`
	RealName            string         `json:"real_name"`
	RealNameCamel       string         `json:"realName"`
	Phone               string         `json:"phone"`
	School              string         `json:"school"`
	Company             string         `json:"company"`
	Bio                 string         `json:"bio"`
	TeamName            string         `json:"team_name"`
	TeamNameCamel       string         `json:"teamName"`
	RolePreference      string         `json:"role_preference"`
	RolePreferenceCamel string         `json:"rolePreference"`
	Source              string         `json:"source"`
	Note                string         `json:"note"`
	Extra               map[string]any `json:"extra"`
}

func NewRegistrationHandler(events *repository.EventRepository, registrations *repository.RegistrationRepository, defaultSlug string) *RegistrationHandler {
	return &RegistrationHandler{
		events:        events,
		registrations: registrations,
		defaultSlug:   defaultSlug,
	}
}

func (h *RegistrationHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := httpx.CurrentUser(r)
	if user == nil {
		httpx.Error(w, http.StatusUnauthorized, 40106, "unauthorized")
		return
	}

	req, ok := h.decodeValidatedPayload(w, r)
	if !ok {
		return
	}

	event, ok := h.loadAndValidateEventForSubmit(w, r, req.EventSlug)
	if !ok {
		return
	}

	existing, err := h.registrations.GetByUserAndEvent(r.Context(), user.UID, event.ID)
	if err == nil {
		httpx.Error(w, http.StatusConflict, 40910, "you have already registered for this event")
		_ = existing
		return
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpx.Error(w, http.StatusInternalServerError, 50012, "failed to check existing registration")
		return
	}

	registration, err := h.registrations.Create(r.Context(), repository.CreateRegistrationParams{
		EventID:        event.ID,
		UserID:         user.UID,
		Status:         "submitted",
		RealName:       req.RealName,
		Phone:          req.Phone,
		EmailSnapshot:  user.Email,
		School:         stringPtrOrNil(req.School),
		Company:        stringPtrOrNil(req.Company),
		Bio:            stringPtrOrNil(req.Bio),
		TeamName:       stringPtrOrNil(req.TeamName),
		RolePreference: stringPtrOrNil(req.RolePreference),
		Source:         stringPtrOrNil(req.Source),
		Note:           stringPtrOrNil(req.Note),
		Extra:          req.ExtraJSON,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			httpx.Error(w, http.StatusConflict, 40911, "you have already registered for this event")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50013, "failed to create registration")
		return
	}

	httpx.OK(w, registration, "registration submitted")
}

func (h *RegistrationHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := httpx.CurrentUser(r)
	if user == nil {
		httpx.Error(w, http.StatusUnauthorized, 40110, "unauthorized")
		return
	}

	req, ok := h.decodeValidatedPayload(w, r)
	if !ok {
		return
	}

	event, ok := h.loadEvent(w, r, req.EventSlug, 40403, 50014)
	if !ok {
		return
	}

	registration, err := h.registrations.GetByUserAndEvent(r.Context(), user.UID, event.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40404, "registration not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50015, "failed to load registration")
		return
	}

	if !registrationEditableByUser(registration.Status) {
		httpx.Error(w, http.StatusConflict, 40912, "this registration can no longer be edited")
		return
	}

	if !eventAllowsRegistration(event.Status, event.RegistrationOpenAt, event.RegistrationCloseAt) {
		httpx.Error(w, http.StatusForbidden, 40310, "registration is not open for this event")
		return
	}

	updatedRegistration, err := h.registrations.UpdateSubmission(r.Context(), repository.UpdateRegistrationSubmissionParams{
		ID:             registration.ID,
		UserID:         user.UID,
		RealName:       req.RealName,
		Phone:          req.Phone,
		EmailSnapshot:  user.Email,
		School:         stringPtrOrNil(req.School),
		Company:        stringPtrOrNil(req.Company),
		Bio:            stringPtrOrNil(req.Bio),
		TeamName:       stringPtrOrNil(req.TeamName),
		RolePreference: stringPtrOrNil(req.RolePreference),
		Source:         stringPtrOrNil(req.Source),
		Note:           stringPtrOrNil(req.Note),
		Extra:          req.ExtraJSON,
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50016, "failed to update registration")
		return
	}

	httpx.OK(w, updatedRegistration, "registration updated")
}

func (h *RegistrationHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	user := httpx.CurrentUser(r)
	if user == nil {
		httpx.Error(w, http.StatusUnauthorized, 40111, "unauthorized")
		return
	}

	eventSlug := readEventSlugFromQuery(r, h.defaultSlug)
	event, ok := h.loadEvent(w, r, eventSlug, 40403, 50014)
	if !ok {
		return
	}

	registration, err := h.registrations.GetByUserAndEvent(r.Context(), user.UID, event.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40404, "registration not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50015, "failed to load registration")
		return
	}

	if !registrationCancellableByUser(registration.Status) {
		httpx.Error(w, http.StatusConflict, 40913, "this registration cannot be cancelled")
		return
	}

	cancelledRegistration, err := h.registrations.Cancel(r.Context(), registration.ID, user.UID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50017, "failed to cancel registration")
		return
	}

	httpx.OK(w, cancelledRegistration, "registration cancelled")
}

func (h *RegistrationHandler) Status(w http.ResponseWriter, r *http.Request) {
	user := httpx.CurrentUser(r)
	if user == nil {
		httpx.Error(w, http.StatusUnauthorized, 40107, "unauthorized")
		return
	}

	eventSlug := readEventSlugFromQuery(r, h.defaultSlug)
	event, ok := h.loadEvent(w, r, eventSlug, 40403, 50014)
	if !ok {
		return
	}

	registration, err := h.registrations.GetByUserAndEvent(r.Context(), user.UID, event.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40404, "registration not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50015, "failed to load registration")
		return
	}

	httpx.OK(w, registration, "OK")
}

func (h *RegistrationHandler) Certificate(w http.ResponseWriter, r *http.Request) {
	user := httpx.CurrentUser(r)
	if user == nil {
		httpx.Error(w, http.StatusUnauthorized, 40115, "unauthorized")
		return
	}

	eventSlug := readEventSlugFromQuery(r, h.defaultSlug)
	event, ok := h.loadEvent(w, r, eventSlug, 40403, 50014)
	if !ok {
		return
	}

	registration, err := h.registrations.GetByUserAndEvent(r.Context(), user.UID, event.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40404, "registration not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50015, "failed to load registration")
		return
	}

	filename := fmt.Sprintf("bohack-%d-certificate.txt", registration.ID)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "BoHack 2026 Registration Certificate\n\n")
	_, _ = fmt.Fprintf(w, "Application ID: BH26-%04d\n", registration.ID)
	_, _ = fmt.Fprintf(w, "Name: %s\n", registration.RealName)
	_, _ = fmt.Fprintf(w, "Event: %s\n", registration.EventTitle)
	_, _ = fmt.Fprintf(w, "Status: %s\n", registration.Status)
	_, _ = fmt.Fprintf(w, "Submitted At: %s\n", registration.SubmittedAt.Format(time.RFC3339))
}

type validatedRegistrationPayload struct {
	EventSlug      string
	RealName       string
	Phone          string
	School         string
	Company        string
	Bio            string
	TeamName       string
	RolePreference string
	Source         string
	Note           string
	ExtraJSON      json.RawMessage
}

func (h *RegistrationHandler) decodeValidatedPayload(w http.ResponseWriter, r *http.Request) (validatedRegistrationPayload, bool) {
	var req registrationPayload
	if !httpx.DecodeJSON(w, r, &req) {
		return validatedRegistrationPayload{}, false
	}

	req.EventSlug = firstNonEmpty(req.EventSlug, req.EventSlugCamel)
	req.RealName = firstNonEmpty(req.RealName, req.RealNameCamel)
	req.TeamName = firstNonEmpty(req.TeamName, req.TeamNameCamel)
	req.RolePreference = firstNonEmpty(req.RolePreference, req.RolePreferenceCamel)

	payload := validatedRegistrationPayload{
		EventSlug:      strings.TrimSpace(req.EventSlug),
		RealName:       strings.TrimSpace(req.RealName),
		Phone:          strings.TrimSpace(req.Phone),
		School:         strings.TrimSpace(req.School),
		Company:        strings.TrimSpace(req.Company),
		Bio:            strings.TrimSpace(req.Bio),
		TeamName:       strings.TrimSpace(req.TeamName),
		RolePreference: strings.TrimSpace(req.RolePreference),
		Source:         strings.TrimSpace(req.Source),
		Note:           strings.TrimSpace(req.Note),
	}
	if payload.EventSlug == "" {
		payload.EventSlug = h.defaultSlug
	}

	switch {
	case payload.RealName == "":
		httpx.Error(w, http.StatusBadRequest, 42210, "real_name is required")
		return validatedRegistrationPayload{}, false
	case payload.Phone == "":
		httpx.Error(w, http.StatusBadRequest, 42211, "phone is required")
		return validatedRegistrationPayload{}, false
	case tooLong(payload.RealName, 100):
		httpx.Error(w, http.StatusBadRequest, 42213, "real_name must be 100 characters or fewer")
		return validatedRegistrationPayload{}, false
	case tooLong(payload.Phone, 32):
		httpx.Error(w, http.StatusBadRequest, 42214, "phone must be 32 characters or fewer")
		return validatedRegistrationPayload{}, false
	case tooLong(payload.School, 255):
		httpx.Error(w, http.StatusBadRequest, 42216, "school must be 255 characters or fewer")
		return validatedRegistrationPayload{}, false
	case tooLong(payload.Company, 255):
		httpx.Error(w, http.StatusBadRequest, 42217, "company must be 255 characters or fewer")
		return validatedRegistrationPayload{}, false
	case tooLong(payload.TeamName, 255):
		httpx.Error(w, http.StatusBadRequest, 42218, "team_name must be 255 characters or fewer")
		return validatedRegistrationPayload{}, false
	case tooLong(payload.RolePreference, 50):
		httpx.Error(w, http.StatusBadRequest, 42219, "role_preference must be 50 characters or fewer")
		return validatedRegistrationPayload{}, false
	case tooLong(payload.Source, 100):
		httpx.Error(w, http.StatusBadRequest, 42223, "source must be 100 characters or fewer")
		return validatedRegistrationPayload{}, false
	case tooLong(payload.Note, 5000):
		httpx.Error(w, http.StatusBadRequest, 42215, "note must be 5000 characters or fewer")
		return validatedRegistrationPayload{}, false
	}

	extra := req.Extra
	if extra == nil {
		extra = map[string]any{}
	}
	extraJSON, err := json.Marshal(extra)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, 42212, "invalid extra payload")
		return validatedRegistrationPayload{}, false
	}
	payload.ExtraJSON = extraJSON

	return payload, true
}

func (h *RegistrationHandler) loadAndValidateEventForSubmit(w http.ResponseWriter, r *http.Request, eventSlug string) (*models.Event, bool) {
	event, ok := h.loadEvent(w, r, eventSlug, 40402, 50011)
	if !ok {
		return nil, false
	}
	if !eventAllowsRegistration(event.Status, event.RegistrationOpenAt, event.RegistrationCloseAt) {
		httpx.Error(w, http.StatusForbidden, 40310, "registration is not open for this event")
		return nil, false
	}
	return event, true
}

func (h *RegistrationHandler) loadEvent(w http.ResponseWriter, r *http.Request, eventSlug string, notFoundCode, serverErrorCode int) (*models.Event, bool) {
	event, err := h.events.GetBySlug(r.Context(), eventSlug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, notFoundCode, "event not found")
			return nil, false
		}
		httpx.Error(w, http.StatusInternalServerError, serverErrorCode, "failed to load event")
		return nil, false
	}
	return event, true
}

func eventAllowsRegistration(status string, openAt, closeAt *time.Time) bool {
	if status != "published" {
		return false
	}

	now := time.Now().UTC()
	if openAt != nil && now.Before(*openAt) {
		return false
	}
	if closeAt != nil && now.After(*closeAt) {
		return false
	}
	return true
}

func registrationEditableByUser(status string) bool {
	switch status {
	case "draft", "submitted", "rejected":
		return true
	default:
		return false
	}
}

func registrationCancellableByUser(status string) bool {
	switch status {
	case "draft", "submitted", "under_review", "rejected":
		return true
	default:
		return false
	}
}

func registrationAllowsAttachmentChanges(status string) bool {
	switch status {
	case "draft", "submitted", "under_review", "rejected":
		return true
	default:
		return false
	}
}

func tooLong(value string, maxRunes int) bool {
	return utf8.RuneCountInString(value) > maxRunes
}

func readEventSlugFromQuery(r *http.Request, fallback string) string {
	eventSlug := strings.TrimSpace(r.URL.Query().Get("event_slug"))
	if eventSlug == "" {
		eventSlug = strings.TrimSpace(r.URL.Query().Get("eventSlug"))
	}
	if eventSlug == "" {
		eventSlug = fallback
	}
	return eventSlug
}

func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
