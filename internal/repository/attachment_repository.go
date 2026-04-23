package repository

import (
	"context"
	"database/sql"
	"time"

	"bohack_backend_go/internal/models"
)

type AttachmentRepository struct {
	db *sql.DB
}

type CreateAttachmentParams struct {
	RegistrationID int64
	Kind           string
	StoragePath    string
	FileName       string
	MimeType       string
	FileSize       int64
}

func NewAttachmentRepository(db *sql.DB) *AttachmentRepository {
	return &AttachmentRepository{db: db}
}

func (r *AttachmentRepository) Create(ctx context.Context, params CreateAttachmentParams) (*models.RegistrationAttachment, error) {
	now := time.Now().UTC()

	var id int64
	if err := r.db.QueryRowContext(
		ctx,
		`
		INSERT INTO event_registration_attachments (
			registration_id, kind, file_url, file_name, mime_type, file_size, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		)
		RETURNING id
		`,
		params.RegistrationID,
		params.Kind,
		params.StoragePath,
		params.FileName,
		params.MimeType,
		params.FileSize,
		now,
	).Scan(&id); err != nil {
		return nil, err
	}

	return r.GetByID(ctx, id)
}

func (r *AttachmentRepository) GetByID(ctx context.Context, id int64) (*models.RegistrationAttachment, error) {
	row := r.db.QueryRowContext(ctx, attachmentSelectByClause(`era.id = $1`), id)
	return scanAttachment(row)
}

func (r *AttachmentRepository) ListByRegistration(ctx context.Context, registrationID int64) ([]*models.RegistrationAttachment, error) {
	rows, err := r.db.QueryContext(
		ctx,
		attachmentSelectBase()+`
		WHERE era.registration_id = $1
		ORDER BY era.created_at ASC, era.id ASC
		`,
		registrationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*models.RegistrationAttachment, 0)
	for rows.Next() {
		item, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *AttachmentRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM event_registration_attachments WHERE id = $1`, id)
	return err
}

func attachmentSelectBase() string {
	return `
		SELECT
			era.id,
			era.registration_id,
			er.user_id,
			era.kind,
			era.file_url,
			era.file_name,
			era.mime_type,
			era.file_size,
			era.created_at
		FROM event_registration_attachments era
		INNER JOIN event_registrations er ON er.id = era.registration_id
	`
}

func attachmentSelectByClause(where string) string {
	return attachmentSelectBase() + `
		WHERE ` + where + `
		LIMIT 1
	`
}

func scanAttachment(scanner rowScanner) (*models.RegistrationAttachment, error) {
	var attachment models.RegistrationAttachment
	if err := scanner.Scan(
		&attachment.ID,
		&attachment.RegistrationID,
		&attachment.UserID,
		&attachment.Kind,
		&attachment.StoragePath,
		&attachment.FileName,
		&attachment.MimeType,
		&attachment.FileSize,
		&attachment.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &attachment, nil
}
