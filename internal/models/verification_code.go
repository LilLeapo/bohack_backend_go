package models

import "time"

type VerificationCode struct {
	ID         int64     `json:"id"         gorm:"column:id;primaryKey;autoIncrement"`
	Email      string    `json:"email"      gorm:"column:email;type:varchar(255);not null;uniqueIndex:uniq_verification_email_type,priority:1;index:idx_verification_codes_email"`
	CodeType   string    `json:"codeType"   gorm:"column:code_type;type:varchar(50);not null;default:'register';uniqueIndex:uniq_verification_email_type,priority:2;index:idx_verification_codes_code_type"`
	Code       string    `json:"-"          gorm:"column:code;type:char(6);not null"`
	ExpiresAt  time.Time `json:"expiresAt"  gorm:"column:expires_at;not null;index:idx_verification_codes_expires_at"`
	LastSendAt time.Time `json:"lastSendAt" gorm:"column:last_send_at;not null"`
}

func (VerificationCode) TableName() string {
	return "verification_codes"
}
