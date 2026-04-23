package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"bohack_backend_go/internal/httpx"
	"bohack_backend_go/internal/models"
	"bohack_backend_go/internal/repository"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var eventSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:[-_][a-z0-9]+)*$`)

type AdminEventHandler struct {
	events *repository.EventRepository
}

type createEventRequest struct {
	Slug                     string  `json:"slug"`
	Title                    string  `json:"title"`
	Status                   string  `json:"status"`
	IsCurrent                *bool   `json:"is_current"`
	IsCurrentCamel           *bool   `json:"isCurrent"`
	RegistrationOpenAt       *string `json:"registration_open_at"`
	RegistrationOpenAtCamel  *string `json:"registrationOpenAt"`
	RegistrationCloseAt      *string `json:"registration_close_at"`
	RegistrationCloseAtCamel *string `json:"registrationCloseAt"`
}

type updateEventRequest struct {
	Slug                     *string `json:"slug"`
	Title                    *string `json:"title"`
	Status                   *string `json:"status"`
	IsCurrent                *bool   `json:"is_current"`
	IsCurrentCamel           *bool   `json:"isCurrent"`
	RegistrationOpenAt       *string `json:"registration_open_at"`
	RegistrationOpenAtCamel  *string `json:"registrationOpenAt"`
	RegistrationCloseAt      *string `json:"registration_close_at"`
	RegistrationCloseAtCamel *string `json:"registrationCloseAt"`
}

type normalizedEventPayload struct {
	Slug                string
	Title               string
	Status              string
	IsCurrent           bool
	RegistrationOpenAt  *time.Time
	RegistrationCloseAt *time.Time
}

func NewAdminEventHandler(events *repository.EventRepository) *AdminEventHandler {
	return &AdminEventHandler{events: events}
}

func (h *AdminEventHandler) List(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("status")))
	if status != "" && !isAllowedEventStatus(status) {
		httpx.Error(w, http.StatusBadRequest, 42241, "invalid event status")
		return
	}

	events, err := h.events.List(r.Context(), status)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50040, "failed to load events")
		return
	}

	httpx.OK(w, events, "OK")
}

func (h *AdminEventHandler) Detail(w http.ResponseWriter, r *http.Request) {
	eventID, ok := readEventID(w, r)
	if !ok {
		return
	}

	event, err := h.events.GetByID(r.Context(), eventID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40412, "event not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50041, "failed to load event")
		return
	}

	httpx.OK(w, event, "OK")
}

func (h *AdminEventHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createEventRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	payload, ok := normalizeCreateEventPayload(w, req)
	if !ok {
		return
	}

	event, err := h.events.Create(r.Context(), repository.CreateEventParams{
		Slug:                payload.Slug,
		Title:               payload.Title,
		Status:              payload.Status,
		IsCurrent:           payload.IsCurrent,
		RegistrationOpenAt:  payload.RegistrationOpenAt,
		RegistrationCloseAt: payload.RegistrationCloseAt,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			httpx.Error(w, http.StatusConflict, 40920, "event slug already exists")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50042, "failed to create event")
		return
	}

	httpx.OK(w, event, "event created")
}

func (h *AdminEventHandler) Update(w http.ResponseWriter, r *http.Request) {
	eventID, ok := readEventID(w, r)
	if !ok {
		return
	}

	existing, err := h.events.GetByID(r.Context(), eventID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40412, "event not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50041, "failed to load event")
		return
	}

	var req updateEventRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	payload, ok := normalizeUpdateEventPayload(w, existing, req)
	if !ok {
		return
	}

	event, err := h.events.Update(r.Context(), repository.UpdateEventParams{
		ID:                  existing.ID,
		Slug:                payload.Slug,
		Title:               payload.Title,
		Status:              payload.Status,
		IsCurrent:           payload.IsCurrent,
		RegistrationOpenAt:  payload.RegistrationOpenAt,
		RegistrationCloseAt: payload.RegistrationCloseAt,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			httpx.Error(w, http.StatusConflict, 40920, "event slug already exists")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50043, "failed to update event")
		return
	}

	httpx.OK(w, event, "event updated")
}

func normalizeCreateEventPayload(w http.ResponseWriter, req createEventRequest) (normalizedEventPayload, bool) {
	payload := normalizedEventPayload{
		Slug:      strings.ToLower(strings.TrimSpace(req.Slug)),
		Title:     strings.TrimSpace(req.Title),
		Status:    strings.ToLower(strings.TrimSpace(req.Status)),
		IsCurrent: firstNonNilBool(req.IsCurrent, req.IsCurrentCamel, false),
	}
	openAt, ok := parseOptionalEventTime(w, firstNonNilStringValue(req.RegistrationOpenAt, req.RegistrationOpenAtCamel), "registration_open_at", 42242)
	if !ok {
		return normalizedEventPayload{}, false
	}
	closeAt, ok := parseOptionalEventTime(w, firstNonNilStringValue(req.RegistrationCloseAt, req.RegistrationCloseAtCamel), "registration_close_at", 42243)
	if !ok {
		return normalizedEventPayload{}, false
	}
	payload.RegistrationOpenAt = openAt
	payload.RegistrationCloseAt = closeAt

	if payload.Status == "" {
		payload.Status = "draft"
	}

	return validateNormalizedEventPayload(w, payload)
}

func normalizeUpdateEventPayload(w http.ResponseWriter, existing *models.Event, req updateEventRequest) (normalizedEventPayload, bool) {
	payload := normalizedEventPayload{
		Slug:                existing.Slug,
		Title:               existing.Title,
		Status:              existing.Status,
		IsCurrent:           existing.IsCurrent,
		RegistrationOpenAt:  existing.RegistrationOpenAt,
		RegistrationCloseAt: existing.RegistrationCloseAt,
	}

	if req.Slug != nil {
		payload.Slug = strings.ToLower(strings.TrimSpace(*req.Slug))
	}
	if req.Title != nil {
		payload.Title = strings.TrimSpace(*req.Title)
	}
	if req.Status != nil {
		payload.Status = strings.ToLower(strings.TrimSpace(*req.Status))
	}
	if req.IsCurrent != nil {
		payload.IsCurrent = *req.IsCurrent
	}
	if req.IsCurrentCamel != nil {
		payload.IsCurrent = *req.IsCurrentCamel
	}
	if req.RegistrationOpenAt != nil || req.RegistrationOpenAtCamel != nil {
		openAt, ok := parseOptionalEventTime(w, firstNonNilStringValue(req.RegistrationOpenAt, req.RegistrationOpenAtCamel), "registration_open_at", 42242)
		if !ok {
			return normalizedEventPayload{}, false
		}
		payload.RegistrationOpenAt = openAt
	}
	if req.RegistrationCloseAt != nil || req.RegistrationCloseAtCamel != nil {
		closeAt, ok := parseOptionalEventTime(w, firstNonNilStringValue(req.RegistrationCloseAt, req.RegistrationCloseAtCamel), "registration_close_at", 42243)
		if !ok {
			return normalizedEventPayload{}, false
		}
		payload.RegistrationCloseAt = closeAt
	}

	return validateNormalizedEventPayload(w, payload)
}

func validateNormalizedEventPayload(w http.ResponseWriter, payload normalizedEventPayload) (normalizedEventPayload, bool) {
	switch {
	case payload.Slug == "":
		httpx.Error(w, http.StatusBadRequest, 42244, "slug is required")
		return normalizedEventPayload{}, false
	case !eventSlugPattern.MatchString(payload.Slug):
		httpx.Error(w, http.StatusBadRequest, 42245, "slug must contain only lowercase letters, numbers, hyphen, or underscore")
		return normalizedEventPayload{}, false
	case len(payload.Slug) > 100:
		httpx.Error(w, http.StatusBadRequest, 42246, "slug must be 100 characters or fewer")
		return normalizedEventPayload{}, false
	case payload.Title == "":
		httpx.Error(w, http.StatusBadRequest, 42247, "title is required")
		return normalizedEventPayload{}, false
	case len(payload.Title) > 255:
		httpx.Error(w, http.StatusBadRequest, 42248, "title must be 255 characters or fewer")
		return normalizedEventPayload{}, false
	case !isAllowedEventStatus(payload.Status):
		httpx.Error(w, http.StatusBadRequest, 42241, "invalid event status")
		return normalizedEventPayload{}, false
	case payload.IsCurrent && payload.Status != "published":
		httpx.Error(w, http.StatusBadRequest, 42249, "current event must be published")
		return normalizedEventPayload{}, false
	case payload.RegistrationOpenAt != nil && payload.RegistrationCloseAt != nil && payload.RegistrationOpenAt.After(*payload.RegistrationCloseAt):
		httpx.Error(w, http.StatusBadRequest, 42250, "registration_open_at must be before registration_close_at")
		return normalizedEventPayload{}, false
	default:
		return payload, true
	}
}

func parseOptionalEventTime(w http.ResponseWriter, raw string, field string, code int) (*time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			utc := parsed.UTC()
			return &utc, true
		}
	}

	httpx.Error(w, http.StatusBadRequest, code, field+" must be a valid RFC3339 or datetime string")
	return nil, false
}

func firstNonNilStringValue(values ...*string) string {
	value, _ := firstNonNilString(values...)
	return value
}

func firstNonNilBool(first, second *bool, fallback bool) bool {
	if first != nil {
		return *first
	}
	if second != nil {
		return *second
	}
	return fallback
}

func readEventID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	rawID := strings.TrimSpace(chi.URLParam(r, "eventID"))
	eventID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || eventID <= 0 {
		httpx.Error(w, http.StatusBadRequest, 42251, "invalid event id")
		return 0, false
	}
	return eventID, true
}

func isAllowedEventStatus(status string) bool {
	switch status {
	case "draft", "published", "archived":
		return true
	default:
		return false
	}
}
