package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"bohack_backend_go/internal/models"
)

type RegistrationRepository struct {
	db *sql.DB
}

type CreateRegistrationParams struct {
	EventID        int64
	UserID         int
	Status         string
	RealName       string
	Phone          string
	EmailSnapshot  string
	School         *string
	Company        *string
	Bio            *string
	TeamName       *string
	RolePreference *string
	Source         *string
	Note           *string
	Extra          json.RawMessage
}

type UpdateRegistrationSubmissionParams struct {
	ID             int64
	UserID         int
	RealName       string
	Phone          string
	EmailSnapshot  string
	School         *string
	Company        *string
	Bio            *string
	TeamName       *string
	RolePreference *string
	Source         *string
	Note           *string
	Extra          json.RawMessage
}

type ReviewRegistrationParams struct {
	ID         int64
	UserID     int
	Status     string
	ReviewedBy int
	ReviewNote *string
}

type ListRegistrationsParams struct {
	EventID  *int64
	Status   string
	Page     int
	PageSize int
}

func NewRegistrationRepository(db *sql.DB) *RegistrationRepository {
	return &RegistrationRepository{db: db}
}

func (r *RegistrationRepository) GetByID(ctx context.Context, id int64) (*models.Registration, error) {
	row := r.db.QueryRowContext(ctx, registrationSelectByClause(`er.id = $1`), id)
	return scanRegistration(row)
}

func (r *RegistrationRepository) GetByUserAndEvent(ctx context.Context, userID int, eventID int64) (*models.Registration, error) {
	row := r.db.QueryRowContext(ctx, registrationSelectByClause(`er.user_id = $1 AND er.event_id = $2`), userID, eventID)
	return scanRegistration(row)
}

func (r *RegistrationRepository) Create(ctx context.Context, params CreateRegistrationParams) (*models.Registration, error) {
	now := time.Now().UTC()
	if len(params.Extra) == 0 || string(params.Extra) == "null" {
		params.Extra = json.RawMessage(`{}`)
	}

	var registrationID int64
	if err := r.db.QueryRowContext(
		ctx,
		`
		INSERT INTO event_registrations (
			event_id, user_id, status, real_name, phone, email_snapshot,
			school, company, bio, team_name, role_preference, source,
			note, extra, submitted_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15, $15, $15
		)
		RETURNING id
		`,
		params.EventID,
		params.UserID,
		params.Status,
		params.RealName,
		params.Phone,
		params.EmailSnapshot,
		params.School,
		params.Company,
		params.Bio,
		params.TeamName,
		params.RolePreference,
		params.Source,
		params.Note,
		params.Extra,
		now,
	).Scan(&registrationID); err != nil {
		return nil, err
	}

	return r.GetByID(ctx, registrationID)
}

func (r *RegistrationRepository) UpdateSubmission(ctx context.Context, params UpdateRegistrationSubmissionParams) (*models.Registration, error) {
	now := time.Now().UTC()
	if len(params.Extra) == 0 || string(params.Extra) == "null" {
		params.Extra = json.RawMessage(`{}`)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`
		UPDATE event_registrations
		SET status = 'submitted',
			real_name = $1,
			phone = $2,
			email_snapshot = $3,
			school = $4,
			company = $5,
			bio = $6,
			team_name = $7,
			role_preference = $8,
			source = $9,
			note = $10,
			extra = $11,
			submitted_at = $12,
			reviewed_at = NULL,
			reviewed_by = NULL,
			review_note = NULL,
			updated_at = $12
		WHERE id = $13
		`,
		params.RealName,
		params.Phone,
		params.EmailSnapshot,
		params.School,
		params.Company,
		params.Bio,
		params.TeamName,
		params.RolePreference,
		params.Source,
		params.Note,
		params.Extra,
		now,
		params.ID,
	); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(
		ctx,
		`
		UPDATE users
		SET role = 'visitor',
			updated_at = $1
		WHERE uid = $2
		  AND is_admin = false
		`,
		now,
		params.UserID,
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.GetByID(ctx, params.ID)
}

func (r *RegistrationRepository) Cancel(ctx context.Context, id int64, userID int) (*models.Registration, error) {
	now := time.Now().UTC()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`
		UPDATE event_registrations
		SET status = 'cancelled',
			reviewed_at = NULL,
			reviewed_by = NULL,
			review_note = NULL,
			updated_at = $1
		WHERE id = $2
		`,
		now,
		id,
	); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(
		ctx,
		`
		UPDATE users
		SET role = 'visitor',
			updated_at = $1
		WHERE uid = $2
		  AND is_admin = false
		`,
		now,
		userID,
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.GetByID(ctx, id)
}

func (r *RegistrationRepository) Review(ctx context.Context, params ReviewRegistrationParams) (*models.Registration, error) {
	now := time.Now().UTC()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if params.Status == "submitted" {
		if _, err := tx.ExecContext(
			ctx,
			`
			UPDATE event_registrations
			SET status = $1,
				reviewed_at = NULL,
				reviewed_by = NULL,
				review_note = NULL,
				updated_at = $2
			WHERE id = $3
			`,
			params.Status,
			now,
			params.ID,
		); err != nil {
			return nil, err
		}
	} else {
		if _, err := tx.ExecContext(
			ctx,
			`
			UPDATE event_registrations
			SET status = $1,
				reviewed_at = $2,
				reviewed_by = $3,
				review_note = $4,
				updated_at = $2
			WHERE id = $5
			`,
			params.Status,
			now,
			params.ReviewedBy,
			params.ReviewNote,
			params.ID,
		); err != nil {
			return nil, err
		}
	}

	userRole := roleForRegistrationStatus(params.Status)
	if _, err := tx.ExecContext(
		ctx,
		`
		UPDATE users
		SET role = $1,
			updated_at = $2
		WHERE uid = $3
		  AND is_admin = false
		`,
		userRole,
		now,
		params.UserID,
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.GetByID(ctx, params.ID)
}

func (r *RegistrationRepository) List(ctx context.Context, params ListRegistrationsParams) ([]*models.Registration, int, error) {
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}

	whereClauses := []string{"1 = 1"}
	args := make([]any, 0, 4)

	if params.EventID != nil {
		args = append(args, *params.EventID)
		whereClauses = append(whereClauses, fmt.Sprintf("er.event_id = $%d", len(args)))
	}
	if params.Status != "" {
		args = append(args, params.Status)
		whereClauses = append(whereClauses, fmt.Sprintf("er.status = $%d", len(args)))
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	var total int
	countQuery := `SELECT COUNT(1) FROM event_registrations er WHERE ` + whereSQL
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.PageSize
	args = append(args, params.PageSize, offset)
	listQuery := registrationSelectBase() +
		` WHERE ` + whereSQL +
		fmt.Sprintf(" ORDER BY er.created_at DESC, er.id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	rows, err := r.db.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]*models.Registration, 0, params.PageSize)
	for rows.Next() {
		item, err := scanRegistration(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func registrationSelectBase() string {
	return `
		SELECT
			er.id,
			er.event_id,
			e.slug,
			e.title,
			er.user_id,
			u.username,
			er.status,
			er.real_name,
			er.phone,
			er.email_snapshot,
			er.school,
			er.company,
			er.bio,
			er.team_name,
			er.role_preference,
			er.source,
			er.note,
			er.extra,
			er.submitted_at,
			er.reviewed_at,
			er.reviewed_by,
			er.review_note,
			er.created_at,
			er.updated_at
		FROM event_registrations er
		INNER JOIN events e ON e.id = er.event_id
		LEFT JOIN users u ON u.uid = er.user_id
	`
}

func registrationSelectByClause(where string) string {
	return registrationSelectBase() + `
		WHERE ` + where + `
		LIMIT 1
	`
}

func scanRegistration(scanner rowScanner) (*models.Registration, error) {
	var registration models.Registration
	var username sql.NullString
	var school sql.NullString
	var company sql.NullString
	var bio sql.NullString
	var teamName sql.NullString
	var rolePreference sql.NullString
	var source sql.NullString
	var note sql.NullString
	var reviewedAt sql.NullTime
	var reviewedBy sql.NullInt64
	var reviewNote sql.NullString

	if err := scanner.Scan(
		&registration.ID,
		&registration.EventID,
		&registration.EventSlug,
		&registration.EventTitle,
		&registration.UserID,
		&username,
		&registration.Status,
		&registration.RealName,
		&registration.Phone,
		&registration.EmailSnapshot,
		&school,
		&company,
		&bio,
		&teamName,
		&rolePreference,
		&source,
		&note,
		&registration.Extra,
		&registration.SubmittedAt,
		&reviewedAt,
		&reviewedBy,
		&reviewNote,
		&registration.CreatedAt,
		&registration.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if len(registration.Extra) == 0 {
		registration.Extra = json.RawMessage(`{}`)
	}
	if username.Valid {
		registration.Username = &username.String
	}
	if school.Valid {
		registration.School = &school.String
	}
	if company.Valid {
		registration.Company = &company.String
	}
	if bio.Valid {
		registration.Bio = &bio.String
	}
	if teamName.Valid {
		registration.TeamName = &teamName.String
	}
	if rolePreference.Valid {
		registration.RolePreference = &rolePreference.String
	}
	if source.Valid {
		registration.Source = &source.String
	}
	if note.Valid {
		registration.Note = &note.String
	}
	if reviewedAt.Valid {
		registration.ReviewedAt = &reviewedAt.Time
	}
	if reviewedBy.Valid {
		value := int(reviewedBy.Int64)
		registration.ReviewedBy = &value
	}
	if reviewNote.Valid {
		registration.ReviewNote = &reviewNote.String
	}

	return &registration, nil
}

func roleForRegistrationStatus(status string) string {
	switch status {
	case "approved":
		return "contestant"
	case "rejected":
		return "experiencer"
	default:
		return "visitor"
	}
}
