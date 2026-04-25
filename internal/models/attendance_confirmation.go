package models

import "time"

type AttendanceConfirmation struct {
	ID             int64      `json:"id"             gorm:"column:id;primaryKey;autoIncrement"`
	RegistrationID int64      `json:"registrationId" gorm:"column:registration_id;not null;index:idx_attendance_confirmations_registration_id"`
	UserID         int        `json:"userId"         gorm:"column:user_id;not null;index:idx_attendance_confirmations_user_id"`
	TokenHash      string     `json:"-"              gorm:"column:token_hash;type:varchar(64);not null;uniqueIndex:uniq_attendance_confirmation_token_hash"`
	Status         string     `json:"status"         gorm:"column:status;type:varchar(20);not null;default:'pending';index:idx_attendance_confirmations_status"`
	ExpiresAt      time.Time  `json:"expiresAt"      gorm:"column:expires_at;not null;index:idx_attendance_confirmations_expires_at"`
	SentAt         time.Time  `json:"sentAt"         gorm:"column:sent_at;not null"`
	RespondedAt    *time.Time `json:"respondedAt,omitempty" gorm:"column:responded_at"`
	CreatedAt      time.Time  `json:"createdAt"      gorm:"column:created_at;not null"`
	UpdatedAt      time.Time  `json:"updatedAt"      gorm:"column:updated_at;not null"`
}

func (AttendanceConfirmation) TableName() string {
	return "attendance_confirmations"
}
