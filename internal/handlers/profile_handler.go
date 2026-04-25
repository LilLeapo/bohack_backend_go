package handlers

import (
	"net/http"
	"strings"

	"bohack_backend_go/internal/httpx"
	"bohack_backend_go/internal/repository"
)

type ProfileHandler struct {
	users *repository.UserRepository
}

type updateProfileRequest struct {
	AvatarURL      *string `json:"avatar_url"`
	AvatarURLCamel *string `json:"avatarUrl"`
	Bio            *string `json:"bio"`
	Phone          *string `json:"phone"`
}

func NewProfileHandler(users *repository.UserRepository) *ProfileHandler {
	return &ProfileHandler{users: users}
}

func (h *ProfileHandler) Me(w http.ResponseWriter, r *http.Request) {
	user := httpx.CurrentUser(r)
	if user == nil {
		httpx.Error(w, http.StatusUnauthorized, 40105, "unauthorized")
		return
	}
	httpx.OK(w, user, "OK")
}

func (h *ProfileHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := httpx.CurrentUser(r)
	if user == nil {
		httpx.Error(w, http.StatusUnauthorized, 40109, "unauthorized")
		return
	}

	var req updateProfileRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	avatarURL := user.AvatarURL
	if value, ok := firstNonNilString(req.AvatarURL, req.AvatarURLCamel); ok {
		avatarURL = normalizeOptionalString(value)
	}

	bio := user.Bio
	if req.Bio != nil {
		bio = normalizeOptionalString(*req.Bio)
	}

	phone := user.Phone
	if req.Phone != nil {
		phone = normalizeOptionalString(*req.Phone)
	}

	switch {
	case avatarURL != nil && tooLong(*avatarURL, 500):
		httpx.Error(w, http.StatusBadRequest, 42220, "avatar_url must be 500 characters or fewer")
		return
	case bio != nil && tooLong(*bio, 5000):
		httpx.Error(w, http.StatusBadRequest, 42221, "bio must be 5000 characters or fewer")
		return
	case phone != nil && tooLong(*phone, 32):
		httpx.Error(w, http.StatusBadRequest, 42222, "phone must be 32 characters or fewer")
		return
	}

	updatedUser, err := h.users.UpdateProfile(r.Context(), repository.UpdateUserProfileParams{
		UID:       user.UID,
		AvatarURL: avatarURL,
		Bio:       bio,
		Phone:     phone,
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50020, "failed to update profile")
		return
	}

	httpx.OK(w, updatedUser, "profile updated")
}

func firstNonNilString(values ...*string) (string, bool) {
	for _, value := range values {
		if value != nil {
			return *value, true
		}
	}
	return "", false
}

func normalizeOptionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
