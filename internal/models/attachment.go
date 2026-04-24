package models

import "time"

type RegistrationAttachment struct {
	ID             int64     `json:"id"             gorm:"column:id;primaryKey;autoIncrement"`
	RegistrationID int64     `json:"registrationId" gorm:"column:registration_id;not null;index:idx_registration_attachments_registration_id"`
	UserID         int       `json:"userId,omitempty"   gorm:"-"`
	Kind           string    `json:"kind"           gorm:"column:kind;type:varchar(50);not null"`
	FileName       string    `json:"fileName"       gorm:"column:file_name;type:varchar(255);not null"`
	MimeType       string    `json:"mimeType"       gorm:"column:mime_type;type:varchar(100);not null"`
	FileSize       int64     `json:"fileSize"       gorm:"column:file_size;not null;default:0"`
	DownloadURL    string    `json:"downloadUrl,omitempty" gorm:"-"`
	CreatedAt      time.Time `json:"createdAt"      gorm:"column:created_at;not null"`
	StoragePath    string    `json:"-"              gorm:"column:file_url;type:text;not null"`
}

func (RegistrationAttachment) TableName() string {
	return "event_registration_attachments"
}
