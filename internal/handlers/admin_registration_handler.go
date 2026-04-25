package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"bohack_backend_go/internal/httpx"
	"bohack_backend_go/internal/models"
	"bohack_backend_go/internal/repository"

	"github.com/go-chi/chi/v5"
)

type AdminRegistrationHandler struct {
	events        *repository.EventRepository
	registrations *repository.RegistrationRepository
	confirmations *repository.AttendanceConfirmationRepository
}

type reviewRegistrationRequest struct {
	Status          string `json:"status"`
	ReviewNote      string `json:"review_note"`
	ReviewNoteCamel string `json:"reviewNote"`
}

type adminUpdateRegistrationRequest struct {
	RealName            *string         `json:"real_name"`
	RealNameCamel       *string         `json:"realName"`
	Phone               *string         `json:"phone"`
	EmailSnapshot       *string         `json:"email_snapshot"`
	EmailSnapshotCamel  *string         `json:"emailSnapshot"`
	School              *string         `json:"school"`
	Company             *string         `json:"company"`
	Bio                 *string         `json:"bio"`
	TeamName            *string         `json:"team_name"`
	TeamNameCamel       *string         `json:"teamName"`
	RolePreference      *string         `json:"role_preference"`
	RolePreferenceCamel *string         `json:"rolePreference"`
	Source              *string         `json:"source"`
	Note                *string         `json:"note"`
	Extra               *map[string]any `json:"extra"`
}

func NewAdminRegistrationHandler(
	events *repository.EventRepository,
	registrations *repository.RegistrationRepository,
	confirmations *repository.AttendanceConfirmationRepository,
) *AdminRegistrationHandler {
	return &AdminRegistrationHandler{
		events:        events,
		registrations: registrations,
		confirmations: confirmations,
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
	if err := h.attachLatestAttendance(r.Context(), items); err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50035, "failed to load attendance confirmations")
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
	if err := h.attachLatestAttendance(r.Context(), []*models.Registration{registration}); err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50035, "failed to load attendance confirmation")
		return
	}

	httpx.OK(w, registration, "OK")
}

func (h *AdminRegistrationHandler) Update(w http.ResponseWriter, r *http.Request) {
	registrationID, ok := readRegistrationID(w, r)
	if !ok {
		return
	}

	existing, err := h.registrations.GetByID(r.Context(), registrationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40411, "registration not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50032, "failed to load registration")
		return
	}

	var req adminUpdateRegistrationRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	params, ok := normalizeAdminUpdateRegistration(w, existing, req)
	if !ok {
		return
	}

	updatedRegistration, err := h.registrations.AdminUpdate(r.Context(), params)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50034, "failed to update registration")
		return
	}

	httpx.OK(w, updatedRegistration, "registration updated")
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
	if tooLong(req.ReviewNote, 5000) {
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

func (h *AdminRegistrationHandler) attachLatestAttendance(ctx context.Context, registrations []*models.Registration) error {
	if h.confirmations == nil || len(registrations) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(registrations))
	for _, registration := range registrations {
		if registration != nil {
			ids = append(ids, registration.ID)
		}
	}
	latest, err := h.confirmations.LatestByRegistrationIDs(ctx, ids)
	if err != nil {
		return err
	}
	for _, registration := range registrations {
		if registration != nil {
			registration.Attendance = latest[registration.ID]
		}
	}
	return nil
}

func normalizeAdminUpdateRegistration(w http.ResponseWriter, existing *models.Registration, req adminUpdateRegistrationRequest) (repository.AdminUpdateRegistrationParams, bool) {
	realName := existing.RealName
	if value, ok := firstNonNilString(req.RealName, req.RealNameCamel); ok {
		realName = strings.TrimSpace(value)
	}

	phone := existing.Phone
	if req.Phone != nil {
		phone = strings.TrimSpace(*req.Phone)
	}

	emailSnapshot := existing.EmailSnapshot
	if value, ok := firstNonNilString(req.EmailSnapshot, req.EmailSnapshotCamel); ok {
		emailSnapshot = strings.TrimSpace(strings.ToLower(value))
	}

	school := existing.School
	if req.School != nil {
		school = stringPtrOrNil(strings.TrimSpace(*req.School))
	}

	company := existing.Company
	if req.Company != nil {
		company = stringPtrOrNil(strings.TrimSpace(*req.Company))
	}

	bio := existing.Bio
	if req.Bio != nil {
		bio = stringPtrOrNil(strings.TrimSpace(*req.Bio))
	}

	teamName := existing.TeamName
	if value, ok := firstNonNilString(req.TeamName, req.TeamNameCamel); ok {
		teamName = stringPtrOrNil(strings.TrimSpace(value))
	}

	rolePreference := existing.RolePreference
	if value, ok := firstNonNilString(req.RolePreference, req.RolePreferenceCamel); ok {
		rolePreference = stringPtrOrNil(strings.TrimSpace(value))
	}

	source := existing.Source
	if req.Source != nil {
		source = stringPtrOrNil(strings.TrimSpace(*req.Source))
	}

	note := existing.Note
	if req.Note != nil {
		note = stringPtrOrNil(strings.TrimSpace(*req.Note))
	}

	extraJSON := json.RawMessage(existing.Extra)
	if req.Extra != nil {
		marshaled, err := json.Marshal(req.Extra)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, 42212, "invalid extra payload")
			return repository.AdminUpdateRegistrationParams{}, false
		}
		extraJSON = marshaled
	}

	switch {
	case realName == "":
		httpx.Error(w, http.StatusBadRequest, 42210, "real_name is required")
		return repository.AdminUpdateRegistrationParams{}, false
	case phone == "":
		httpx.Error(w, http.StatusBadRequest, 42211, "phone is required")
		return repository.AdminUpdateRegistrationParams{}, false
	case emailSnapshot == "":
		httpx.Error(w, http.StatusBadRequest, 42235, "email_snapshot is required")
		return repository.AdminUpdateRegistrationParams{}, false
	case tooLong(realName, 100):
		httpx.Error(w, http.StatusBadRequest, 42213, "real_name must be 100 characters or fewer")
		return repository.AdminUpdateRegistrationParams{}, false
	case tooLong(phone, 32):
		httpx.Error(w, http.StatusBadRequest, 42214, "phone must be 32 characters or fewer")
		return repository.AdminUpdateRegistrationParams{}, false
	case tooLong(emailSnapshot, 255):
		httpx.Error(w, http.StatusBadRequest, 42236, "email_snapshot must be 255 characters or fewer")
		return repository.AdminUpdateRegistrationParams{}, false
	case school != nil && tooLong(*school, 255):
		httpx.Error(w, http.StatusBadRequest, 42216, "school must be 255 characters or fewer")
		return repository.AdminUpdateRegistrationParams{}, false
	case company != nil && tooLong(*company, 255):
		httpx.Error(w, http.StatusBadRequest, 42217, "company must be 255 characters or fewer")
		return repository.AdminUpdateRegistrationParams{}, false
	case teamName != nil && tooLong(*teamName, 255):
		httpx.Error(w, http.StatusBadRequest, 42218, "team_name must be 255 characters or fewer")
		return repository.AdminUpdateRegistrationParams{}, false
	case rolePreference != nil && tooLong(*rolePreference, 50):
		httpx.Error(w, http.StatusBadRequest, 42219, "role_preference must be 50 characters or fewer")
		return repository.AdminUpdateRegistrationParams{}, false
	case source != nil && tooLong(*source, 100):
		httpx.Error(w, http.StatusBadRequest, 42223, "source must be 100 characters or fewer")
		return repository.AdminUpdateRegistrationParams{}, false
	case note != nil && tooLong(*note, 5000):
		httpx.Error(w, http.StatusBadRequest, 42215, "note must be 5000 characters or fewer")
		return repository.AdminUpdateRegistrationParams{}, false
	}

	return repository.AdminUpdateRegistrationParams{
		ID:             existing.ID,
		RealName:       realName,
		Phone:          phone,
		EmailSnapshot:  emailSnapshot,
		School:         school,
		Company:        company,
		Bio:            bio,
		TeamName:       teamName,
		RolePreference: rolePreference,
		Source:         source,
		Note:           note,
		Extra:          extraJSON,
	}, true
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
