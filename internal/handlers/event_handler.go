package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"bohack_backend_go/internal/httpx"
	"bohack_backend_go/internal/repository"

	"github.com/go-chi/chi/v5"
)

type EventHandler struct {
	events      *repository.EventRepository
	defaultSlug string
}

func NewEventHandler(events *repository.EventRepository, defaultSlug string) *EventHandler {
	return &EventHandler{
		events:      events,
		defaultSlug: defaultSlug,
	}
}

func (h *EventHandler) ListPublic(w http.ResponseWriter, r *http.Request) {
	events, err := h.events.ListPublic(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50008, "failed to load events")
		return
	}
	httpx.OK(w, events, "OK")
}

func (h *EventHandler) GetCurrent(w http.ResponseWriter, r *http.Request) {
	event, err := h.events.GetCurrent(r.Context(), h.defaultSlug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40401, "event not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50010, "failed to load event")
		return
	}
	httpx.OK(w, event, "OK")
}

func (h *EventHandler) GetPublicBySlug(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(chi.URLParam(r, "slug"))
	if slug == "" {
		httpx.Error(w, http.StatusBadRequest, 42240, "event slug is required")
		return
	}

	event, err := h.events.GetPublicBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40405, "event not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50018, "failed to load event")
		return
	}

	httpx.OK(w, event, "OK")
}
