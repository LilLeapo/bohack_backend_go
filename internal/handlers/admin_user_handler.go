package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"

	"bohack_backend_go/internal/httpx"
	"bohack_backend_go/internal/models"
	"bohack_backend_go/internal/repository"

	"github.com/go-chi/chi/v5"
)

type AdminUserHandler struct {
	users *repository.UserRepository
}

type adminUpdateUserRequest struct {
	Username       *string  `json:"username"`
	Email          *string  `json:"email"`
	AvatarURL      *string  `json:"avatar_url"`
	AvatarURLCamel *string  `json:"avatarUrl"`
	Bio            *string  `json:"bio"`
	Phone          *string  `json:"phone"`
	IsAdmin        *bool    `json:"is_admin"`
	IsAdminCamel   *bool    `json:"isAdmin"`
	Role           *string  `json:"role"`
	BKBalance      *float64 `json:"bk_balance"`
	BKBalanceCamel *float64 `json:"bkBalance"`
	TeamID         *int     `json:"team_id"`
	TeamIDCamel    *int     `json:"teamId"`
}

func NewAdminUserHandler(users *repository.UserRepository) *AdminUserHandler {
	return &AdminUserHandler{users: users}
}

func (h *AdminUserHandler) List(w http.ResponseWriter, r *http.Request) {
	page, pageSize, ok := readPagination(w, r)
	if !ok {
		return
	}

	role := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("role")))
	if role != "" && !isAllowedUserRole(role) {
		httpx.Error(w, http.StatusBadRequest, 42270, "invalid user role")
		return
	}

	items, total, err := h.users.List(r.Context(), repository.ListUsersParams{
		Query:    strings.TrimSpace(r.URL.Query().Get("q")),
		Role:     role,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50070, "failed to list users")
		return
	}

	httpx.OK(w, map[string]any{
		"items":    items,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	}, "OK")
}

func (h *AdminUserHandler) Detail(w http.ResponseWriter, r *http.Request) {
	uid, ok := readUserID(w, r)
	if !ok {
		return
	}

	user, err := h.users.GetByID(r.Context(), uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40470, "user not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50071, "failed to load user")
		return
	}

	httpx.OK(w, user, "OK")
}

func (h *AdminUserHandler) Update(w http.ResponseWriter, r *http.Request) {
	uid, ok := readUserID(w, r)
	if !ok {
		return
	}

	existing, err := h.users.GetByID(r.Context(), uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40470, "user not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50071, "failed to load user")
		return
	}

	var req adminUpdateUserRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	params, ok := normalizeAdminUpdateUser(w, existing, req)
	if !ok {
		return
	}

	updatedUser, err := h.users.AdminUpdate(r.Context(), params)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50072, "failed to update user")
		return
	}

	httpx.OK(w, updatedUser, "user updated")
}

func normalizeAdminUpdateUser(w http.ResponseWriter, existing *models.User, req adminUpdateUserRequest) (repository.AdminUpdateUserParams, bool) {
	username := existing.Username
	if req.Username != nil {
		username = strings.TrimSpace(*req.Username)
	}

	email := existing.Email
	if req.Email != nil {
		email = strings.TrimSpace(strings.ToLower(*req.Email))
	}

	avatarURL := existing.AvatarURL
	if value, ok := firstNonNilString(req.AvatarURL, req.AvatarURLCamel); ok {
		avatarURL = normalizeOptionalString(value)
	}

	bio := existing.Bio
	if req.Bio != nil {
		bio = normalizeOptionalString(*req.Bio)
	}

	phone := existing.Phone
	if req.Phone != nil {
		phone = normalizeOptionalString(*req.Phone)
	}

	isAdmin := existing.IsAdmin
	if req.IsAdmin != nil {
		isAdmin = *req.IsAdmin
	}
	if req.IsAdminCamel != nil {
		isAdmin = *req.IsAdminCamel
	}

	role := existing.Role
	if req.Role != nil {
		role = strings.TrimSpace(strings.ToLower(*req.Role))
	}

	bkBalance := existing.BKBalance
	if req.BKBalance != nil {
		bkBalance = *req.BKBalance
	}
	if req.BKBalanceCamel != nil {
		bkBalance = *req.BKBalanceCamel
	}

	teamID := existing.TeamID
	if req.TeamID != nil {
		teamID = req.TeamID
	}
	if req.TeamIDCamel != nil {
		teamID = req.TeamIDCamel
	}

	switch {
	case username == "" || tooLong(username, 50):
		httpx.Error(w, http.StatusBadRequest, 42271, "username must be between 1 and 50 characters")
		return repository.AdminUpdateUserParams{}, false
	case email == "":
		httpx.Error(w, http.StatusBadRequest, 42272, "email is required")
		return repository.AdminUpdateUserParams{}, false
	case tooLong(email, 255):
		httpx.Error(w, http.StatusBadRequest, 42273, "email must be 255 characters or fewer")
		return repository.AdminUpdateUserParams{}, false
	case avatarURL != nil && tooLong(*avatarURL, 500):
		httpx.Error(w, http.StatusBadRequest, 42220, "avatar_url must be 500 characters or fewer")
		return repository.AdminUpdateUserParams{}, false
	case bio != nil && tooLong(*bio, 5000):
		httpx.Error(w, http.StatusBadRequest, 42221, "bio must be 5000 characters or fewer")
		return repository.AdminUpdateUserParams{}, false
	case phone != nil && tooLong(*phone, 32):
		httpx.Error(w, http.StatusBadRequest, 42222, "phone must be 32 characters or fewer")
		return repository.AdminUpdateUserParams{}, false
	case !isAllowedUserRole(role):
		httpx.Error(w, http.StatusBadRequest, 42270, "invalid user role")
		return repository.AdminUpdateUserParams{}, false
	case bkBalance < 0:
		httpx.Error(w, http.StatusBadRequest, 42274, "bk_balance must be 0 or greater")
		return repository.AdminUpdateUserParams{}, false
	case teamID != nil && *teamID <= 0:
		httpx.Error(w, http.StatusBadRequest, 42275, "team_id must be positive")
		return repository.AdminUpdateUserParams{}, false
	}
	if _, err := mail.ParseAddress(email); err != nil {
		httpx.Error(w, http.StatusBadRequest, 42206, "invalid email address")
		return repository.AdminUpdateUserParams{}, false
	}

	return repository.AdminUpdateUserParams{
		UID:       existing.UID,
		Username:  username,
		Email:     email,
		AvatarURL: avatarURL,
		Bio:       bio,
		Phone:     phone,
		IsAdmin:   isAdmin,
		Role:      role,
		BKBalance: bkBalance,
		TeamID:    teamID,
	}, true
}

func readUserID(w http.ResponseWriter, r *http.Request) (int, bool) {
	rawID := strings.TrimSpace(chi.URLParam(r, "userID"))
	uid, err := strconv.Atoi(rawID)
	if err != nil || uid <= 0 {
		httpx.Error(w, http.StatusBadRequest, 42276, "invalid user id")
		return 0, false
	}
	return uid, true
}

func isAllowedUserRole(role string) bool {
	switch role {
	case "visitor", "contestant", "experiencer", "admin":
		return true
	default:
		return false
	}
}
