package repository

import (
	"context"
	"time"

	"bohack_backend_go/internal/models"

	"gorm.io/gorm"
)

type AttendanceConfirmationRepository struct {
	db *gorm.DB
}

type CreateAttendanceConfirmationParams struct {
	RegistrationID int64
	UserID         int
	TokenHash      string
	ExpiresAt      time.Time
}

func NewAttendanceConfirmationRepository(db *gorm.DB) *AttendanceConfirmationRepository {
	return &AttendanceConfirmationRepository{db: db}
}

func (r *AttendanceConfirmationRepository) Create(ctx context.Context, params CreateAttendanceConfirmationParams) (*models.AttendanceConfirmation, error) {
	now := time.Now().UTC()
	item := models.AttendanceConfirmation{
		RegistrationID: params.RegistrationID,
		UserID:         params.UserID,
		TokenHash:      params.TokenHash,
		Status:         "pending",
		ExpiresAt:      params.ExpiresAt,
		SentAt:         now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := r.db.WithContext(ctx).Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *AttendanceConfirmationRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*models.AttendanceConfirmation, error) {
	var item models.AttendanceConfirmation
	if err := r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).Take(&item).Error; err != nil {
		return nil, translateError(err)
	}
	return &item, nil
}

func (r *AttendanceConfirmationRepository) Respond(ctx context.Context, tokenHash, status string) (*models.AttendanceConfirmation, error) {
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Model(&models.AttendanceConfirmation{}).
		Where("token_hash = ?", tokenHash).
		Updates(map[string]any{
			"status":       status,
			"responded_at": now,
			"updated_at":   now,
		}).Error; err != nil {
		return nil, err
	}
	return r.GetByTokenHash(ctx, tokenHash)
}

func (r *AttendanceConfirmationRepository) LatestByRegistration(ctx context.Context, registrationID int64) (*models.AttendanceConfirmation, error) {
	var item models.AttendanceConfirmation
	if err := r.db.WithContext(ctx).
		Where("registration_id = ?", registrationID).
		Order("created_at DESC, id DESC").
		Take(&item).Error; err != nil {
		return nil, translateError(err)
	}
	return &item, nil
}

func (r *AttendanceConfirmationRepository) LatestByRegistrationIDs(ctx context.Context, registrationIDs []int64) (map[int64]*models.AttendanceConfirmation, error) {
	out := make(map[int64]*models.AttendanceConfirmation)
	if len(registrationIDs) == 0 {
		return out, nil
	}

	var rows []*models.AttendanceConfirmation
	if err := r.db.WithContext(ctx).
		Where("registration_id IN ?", registrationIDs).
		Order("registration_id ASC, created_at DESC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if _, ok := out[row.RegistrationID]; !ok {
			out[row.RegistrationID] = row
		}
	}
	return out, nil
}
