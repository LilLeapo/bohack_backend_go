package models

import "time"

type RegistrationAttachment struct {
	ID             int64     `json:"id"`
	RegistrationID int64     `json:"registrationId"`
	UserID         int       `json:"userId,omitempty"`
	Kind           string    `json:"kind"`
	FileName       string    `json:"fileName"`
	MimeType       string    `json:"mimeType"`
	FileSize       int64     `json:"fileSize"`
	DownloadURL    string    `json:"downloadUrl,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	StoragePath    string    `json:"-"`
}
