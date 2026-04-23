package models

import "time"

type VerificationCode struct {
	ID         int64     `json:"id"`
	Email      string    `json:"email"`
	CodeType   string    `json:"codeType"`
	Code       string    `json:"-"`
	ExpiresAt  time.Time `json:"expiresAt"`
	LastSendAt time.Time `json:"lastSendAt"`
}
