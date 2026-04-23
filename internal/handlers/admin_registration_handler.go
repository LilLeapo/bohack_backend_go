package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"bohack_backend_go/internal/httpx"
	"bohack_backend_go/internal/repository"

	"github.com/go-chi/chi/v5"
)

type AdminRegistrationHandler struct {
	events        *repository.EventRepository
	registrations *repository.RegistrationRepository
}

type reviewRegistrationRequest struct {
	Status          string `json:"status"`
	ReviewNote      string `json:"review_note"`
	ReviewNoteCamel string `json:"reviewNote"`
}

func NewAdminRegistrationHandler(events *repository.EventRepository, registrations *repository.RegistrationRepository) *AdminRegistrationHandler {
	return &AdminRegistrationHandler{
		events:        events,
		registrations: registrations,
	}
}

func (h *AdminRegistrationHandler) List(w http.ResponseWriter, r *http.Request) {
	page, pageSize, ok := readPagination(w, r)
	if !ok {
		return
	}

	status := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("status")))
	if status != "" && !isAllowedAdminRegistrationStatus(status) {
		httpx.Error(w, http.StatusBadRequest, 42230, "invalid registration status")
		return
	}

	var eventID *int64
	eventSlug := readEventSlugFromQuery(r, "")
	if eventSlug != "" {
		event, err := h.events.GetBySlug(r.Context(), eventSlug)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpx.Error(w, http.StatusNotFound, 40410, "event not found")
				return
			}
			httpx.Error(w, http.StatusInternalServerError, 50030, "failed to load event")
			return
		}
		eventID = &event.ID
	}

	items, total, err := h.registrations.List(r.Context(), repository.ListRegistrationsParams{
		EventID:  eventID,
		Status:   status,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50031, "failed to list registrations")
		return
	}

	httpx.OK(w, map[string]any{
		"items":    items,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	}, "OK")
}

func (h *AdminRegistrationHandler) Detail(w http.ResponseWriter, r *http.Request) {
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

	httpx.OK(w, registration, "OK")
}

func (h *AdminRegistrationHandler) Review(w http.ResponseWriter, r *http.Request) {
	admin := httpx.CurrentUser(r)
	if admin == nil {
		httpx.Error(w, http.StatusUnauthorized, 40112, "unauthorized")
		return
	}

	registrationID, ok := readRegistrationID(w, r)
	if !ok {
		return
	}

	var req reviewRegistrationRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	req.Status = strings.TrimSpace(strings.ToLower(req.Status))
	req.ReviewNote = firstNonEmpty(req.ReviewNote, req.ReviewNoteCamel)
	req.ReviewNote = strings.TrimSpace(req.ReviewNote)

	if !isAllowedAdminRegistrationStatus(req.Status) {
		httpx.Error(w, http.StatusBadRequest, 42230, "invalid registration status")
		return
	}
	if len(req.ReviewNote) > 5000 {
		httpx.Error(w, http.StatusBadRequest, 42231, "review_note must be 5000 characters or fewer")
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

	reviewedRegistration, err := h.registrations.Review(r.Context(), repository.ReviewRegistrationParams{
		ID:         registration.ID,
		UserID:     registration.UserID,
		Status:     req.Status,
		ReviewedBy: admin.UID,
		ReviewNote: stringPtrOrNil(req.ReviewNote),
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50033, "failed to review registration")
		return
	}

	httpx.OK(w, reviewedRegistration, "registration reviewed")
}

func readPagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	page := 1
	pageSize := 20

	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			httpx.Error(w, http.StatusBadRequest, 42232, "page must be a positive integer")
			return 0, 0, false
		}
		page = value
	}

	rawPageSize := strings.TrimSpace(r.URL.Query().Get("page_size"))
	if rawPageSize == "" {
		rawPageSize = strings.TrimSpace(r.URL.Query().Get("pageSize"))
	}
	if rawPageSize != "" {
		value, err := strconv.Atoi(rawPageSize)
		if err != nil || value <= 0 || value > 100 {
			httpx.Error(w, http.StatusBadRequest, 42233, "page_size must be between 1 and 100")
			return 0, 0, false
		}
		pageSize = value
	}

	return page, pageSize, true
}

func readRegistrationID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	rawID := strings.TrimSpace(chi.URLParam(r, "registrationID"))
	registrationID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || registrationID <= 0 {
		httpx.Error(w, http.StatusBadRequest, 42234, "invalid registration id")
		return 0, false
	}
	return registrationID, true
}

func isAllowedAdminRegistrationStatus(status string) bool {
	switch status {
	case "submitted", "under_review", "approved", "rejected", "cancelled":
		return true
	default:
		return false
	}
}
