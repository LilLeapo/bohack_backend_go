package repository

import (
	"context"
	"strings"
	"time"

	"bohack_backend_go/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type VerificationCodeRepository struct {
	db *gorm.DB
}

func NewVerificationCodeRepository(db *gorm.DB) *VerificationCodeRepository {
	return &VerificationCodeRepository{db: db}
}

func (r *VerificationCodeRepository) GetByEmailAndType(ctx context.Context, email, codeType string) (*models.VerificationCode, error) {
	var item models.VerificationCode
	if err := r.db.WithContext(ctx).
		Where("email = ? AND code_type = ?", normalizeEmail(email), normalizeCodeType(codeType)).
		Take(&item).Error; err != nil {
		return nil, translateError(err)
	}
	return &item, nil
}

func (r *VerificationCodeRepository) Upsert(ctx context.Context, email, codeType, code string, expiresAt, lastSendAt time.Time) error {
	item := models.VerificationCode{
		Email:      normalizeEmail(email),
		CodeType:   normalizeCodeType(codeType),
		Code:       strings.TrimSpace(code),
		ExpiresAt:  expiresAt,
		LastSendAt: lastSendAt,
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "email"}, {Name: "code_type"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"code",
				"expires_at",
				"last_send_at",
			}),
		}).
		Create(&item).Error
}

func (r *VerificationCodeRepository) DeleteByEmailAndType(ctx context.Context, email, codeType string) error {
	return r.db.WithContext(ctx).
		Where("email = ? AND code_type = ?", normalizeEmail(email), normalizeCodeType(codeType)).
		Delete(&models.VerificationCode{}).Error
}

func normalizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

func normalizeCodeType(codeType string) string {
	return strings.TrimSpace(strings.ToLower(codeType))
}
