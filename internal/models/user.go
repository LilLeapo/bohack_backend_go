package models

import "time"

type User struct {
	UID          int       `json:"uid"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	AvatarURL    *string   `json:"avatarUrl,omitempty"`
	Bio          *string   `json:"bio,omitempty"`
	Phone        *string   `json:"phone,omitempty"`
	IsAdmin      bool      `json:"isAdmin"`
	Role         string    `json:"role"`
	BKBalance    float64   `json:"bkBalance"`
	TeamID       *int      `json:"teamId,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
