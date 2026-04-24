package repository

import (
	"context"
	"time"

	"bohack_backend_go/internal/models"

	"gorm.io/gorm"
)

type AttachmentRepository struct {
	db *gorm.DB
}

type CreateAttachmentParams struct {
	RegistrationID int64
	Kind           string
	StoragePath    string
	FileName       string
	MimeType       string
	FileSize       int64
}

type attachmentRow struct {
	ID             int64     `gorm:"column:id"`
	RegistrationID int64     `gorm:"column:registration_id"`
	UserID         int       `gorm:"column:user_id"`
	Kind           string    `gorm:"column:kind"`
	StoragePath    string    `gorm:"column:file_url"`
	FileName       string    `gorm:"column:file_name"`
	MimeType       string    `gorm:"column:mime_type"`
	FileSize       int64     `gorm:"column:file_size"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func NewAttachmentRepository(db *gorm.DB) *AttachmentRepository {
	return &AttachmentRepository{db: db}
}

func (r *AttachmentRepository) Create(ctx context.Context, params CreateAttachmentParams) (*models.RegistrationAttachment, error) {
	now := time.Now().UTC()
	attachment := models.RegistrationAttachment{
		RegistrationID: params.RegistrationID,
		Kind:           params.Kind,
		StoragePath:    params.StoragePath,
		FileName:       params.FileName,
		MimeType:       params.MimeType,
		FileSize:       params.FileSize,
		CreatedAt:      now,
	}
	if err := r.db.WithContext(ctx).Create(&attachment).Error; err != nil {
		return nil, err
	}
	return r.GetByID(ctx, attachment.ID)
}

func (r *AttachmentRepository) GetByID(ctx context.Context, id int64) (*models.RegistrationAttachment, error) {
	rows, err := r.queryRows(ctx, "era.id = ?", []any{id}, "", 1)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sqlNoRows()
	}
	item := rowToAttachment(rows[0])
	return &item, nil
}

func (r *AttachmentRepository) ListByRegistration(ctx context.Context, registrationID int64) ([]*models.RegistrationAttachment, error) {
	rows, err := r.queryRows(ctx, "era.registration_id = ?", []any{registrationID}, "era.created_at ASC, era.id ASC", 0)
	if err != nil {
		return nil, err
	}
	items := make([]*models.RegistrationAttachment, 0, len(rows))
	for _, row := range rows {
		item := rowToAttachment(row)
		items = append(items, &item)
	}
	return items, nil
}

func (r *AttachmentRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.RegistrationAttachment{}).Error
}

func (r *AttachmentRepository) queryRows(ctx context.Context, where string, args []any, orderBy string, limit int) ([]attachmentRow, error) {
	tx := r.db.WithContext(ctx).
		Table("event_registration_attachments era").
		Select(`
			era.id,
			era.registration_id,
			er.user_id,
			era.kind,
			era.file_url,
			era.file_name,
			era.mime_type,
			era.file_size,
			era.created_at
		`).
		Joins("INNER JOIN event_registrations er ON er.id = era.registration_id")

	if where != "" {
		tx = tx.Where(where, args...)
	}
	if orderBy != "" {
		tx = tx.Order(orderBy)
	}
	if limit > 0 {
		tx = tx.Limit(limit)
	}

	var rows []attachmentRow
	if err := tx.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func rowToAttachment(row attachmentRow) models.RegistrationAttachment {
	return models.RegistrationAttachment{
		ID:             row.ID,
		RegistrationID: row.RegistrationID,
		UserID:         row.UserID,
		Kind:           row.Kind,
		StoragePath:    row.StoragePath,
		FileName:       row.FileName,
		MimeType:       row.MimeType,
		FileSize:       row.FileSize,
		CreatedAt:      row.CreatedAt,
	}
}
