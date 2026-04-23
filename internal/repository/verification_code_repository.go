package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"bohack_backend_go/internal/models"
)

type VerificationCodeRepository struct {
	db *sql.DB
}

func NewVerificationCodeRepository(db *sql.DB) *VerificationCodeRepository {
	return &VerificationCodeRepository{db: db}
}

func (r *VerificationCodeRepository) GetByEmailAndType(ctx context.Context, email, codeType string) (*models.VerificationCode, error) {
	row := r.db.QueryRowContext(
		ctx,
		`
		SELECT id, email, code_type, code, expires_at, last_send_at
		FROM verification_codes
		WHERE email = $1 AND code_type = $2
		LIMIT 1
		`,
		normalizeEmail(email),
		strings.TrimSpace(strings.ToLower(codeType)),
	)

	var item models.VerificationCode
	if err := row.Scan(
		&item.ID,
		&item.Email,
		&item.CodeType,
		&item.Code,
		&item.ExpiresAt,
		&item.LastSendAt,
	); err != nil {
		return nil, err
	}

	return &item, nil
}

func (r *VerificationCodeRepository) Upsert(ctx context.Context, email, codeType, code string, expiresAt, lastSendAt time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`
		INSERT INTO verification_codes (email, code_type, code, expires_at, last_send_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (email, code_type) DO UPDATE
		SET code = EXCLUDED.code,
			expires_at = EXCLUDED.expires_at,
			last_send_at = EXCLUDED.last_send_at
		`,
		normalizeEmail(email),
		strings.TrimSpace(strings.ToLower(codeType)),
		strings.TrimSpace(code),
		expiresAt,
		lastSendAt,
	)
	return err
}

func (r *VerificationCodeRepository) DeleteByEmailAndType(ctx context.Context, email, codeType string) error {
	_, err := r.db.ExecContext(
		ctx,
		`DELETE FROM verification_codes WHERE email = $1 AND code_type = $2`,
		normalizeEmail(email),
		strings.TrimSpace(strings.ToLower(codeType)),
	)
	return err
}

func normalizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}
