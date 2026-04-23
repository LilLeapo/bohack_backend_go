package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"bohack_backend_go/internal/models"
)

type EventRepository struct {
	db *sql.DB
}

type CreateEventParams struct {
	Slug                string
	Title               string
	Status              string
	IsCurrent           bool
	RegistrationOpenAt  *time.Time
	RegistrationCloseAt *time.Time
}

type UpdateEventParams struct {
	ID                  int64
	Slug                string
	Title               string
	Status              string
	IsCurrent           bool
	RegistrationOpenAt  *time.Time
	RegistrationCloseAt *time.Time
}

func NewEventRepository(db *sql.DB) *EventRepository {
	return &EventRepository{db: db}
}

func (r *EventRepository) GetByID(ctx context.Context, id int64) (*models.Event, error) {
	row := r.db.QueryRowContext(ctx, eventSelectByClause(`id = $1`), id)
	return scanEvent(row)
}

func (r *EventRepository) GetBySlug(ctx context.Context, slug string) (*models.Event, error) {
	row := r.db.QueryRowContext(ctx, eventSelectByClause(`slug = $1`), slug)
	return scanEvent(row)
}

func (r *EventRepository) GetPublicBySlug(ctx context.Context, slug string) (*models.Event, error) {
	row := r.db.QueryRowContext(ctx, eventSelectByClause(`slug = $1 AND status = 'published'`), slug)
	return scanEvent(row)
}

func (r *EventRepository) GetCurrent(ctx context.Context, fallbackSlug string) (*models.Event, error) {
	row := r.db.QueryRowContext(ctx, eventSelectByClause(`is_current = true AND status = 'published'`))
	event, err := scanEvent(row)
	if err == nil {
		return event, nil
	}
	if err != nil && !isNoRows(err) {
		return nil, err
	}

	if strings.TrimSpace(fallbackSlug) != "" {
		event, err = r.GetPublicBySlug(ctx, fallbackSlug)
		if err == nil {
			return event, nil
		}
		if err != nil && !isNoRows(err) {
			return nil, err
		}
	}

	row = r.db.QueryRowContext(
		ctx,
		eventSelectBase()+`
		WHERE status = 'published'
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`,
	)
	return scanEvent(row)
}

func (r *EventRepository) ListPublic(ctx context.Context) ([]*models.Event, error) {
	rows, err := r.db.QueryContext(
		ctx,
		eventSelectBase()+`
		WHERE status = 'published'
		ORDER BY is_current DESC, created_at DESC, id DESC
		`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEventRows(rows)
}

func (r *EventRepository) List(ctx context.Context, status string) ([]*models.Event, error) {
	query := eventSelectBase()
	args := make([]any, 0, 1)
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" WHERE status = $%d", len(args))
	}
	query += " ORDER BY is_current DESC, created_at DESC, id DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEventRows(rows)
}

func (r *EventRepository) Create(ctx context.Context, params CreateEventParams) (*models.Event, error) {
	now := time.Now().UTC()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if params.IsCurrent {
		if _, err := tx.ExecContext(ctx, `UPDATE events SET is_current = false WHERE is_current = true`); err != nil {
			return nil, err
		}
	}

	var id int64
	if err := tx.QueryRowContext(
		ctx,
		`
		INSERT INTO events (
			slug, title, status, is_current, registration_open_at, registration_close_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $7
		)
		RETURNING id
		`,
		params.Slug,
		params.Title,
		params.Status,
		params.IsCurrent,
		params.RegistrationOpenAt,
		params.RegistrationCloseAt,
		now,
	).Scan(&id); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.GetByID(ctx, id)
}

func (r *EventRepository) Update(ctx context.Context, params UpdateEventParams) (*models.Event, error) {
	now := time.Now().UTC()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if params.IsCurrent {
		if _, err := tx.ExecContext(ctx, `UPDATE events SET is_current = false WHERE is_current = true AND id <> $1`, params.ID); err != nil {
			return nil, err
		}
	}

	if _, err := tx.ExecContext(
		ctx,
		`
		UPDATE events
		SET slug = $1,
			title = $2,
			status = $3,
			is_current = $4,
			registration_open_at = $5,
			registration_close_at = $6,
			updated_at = $7
		WHERE id = $8
		`,
		params.Slug,
		params.Title,
		params.Status,
		params.IsCurrent,
		params.RegistrationOpenAt,
		params.RegistrationCloseAt,
		now,
		params.ID,
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.GetByID(ctx, params.ID)
}

func eventSelectBase() string {
	return `
		SELECT
			id,
			slug,
			title,
			status,
			is_current,
			registration_open_at,
			registration_close_at,
			created_at,
			updated_at
		FROM events
	`
}

func eventSelectByClause(where string) string {
	return eventSelectBase() + `
		WHERE ` + where + `
		LIMIT 1
	`
}

func scanEvent(scanner rowScanner) (*models.Event, error) {
	var event models.Event
	var openAt sql.NullTime
	var closeAt sql.NullTime

	if err := scanner.Scan(
		&event.ID,
		&event.Slug,
		&event.Title,
		&event.Status,
		&event.IsCurrent,
		&openAt,
		&closeAt,
		&event.CreatedAt,
		&event.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if openAt.Valid {
		event.RegistrationOpenAt = &openAt.Time
	}
	if closeAt.Valid {
		event.RegistrationCloseAt = &closeAt.Time
	}

	return &event, nil
}

func scanEventRows(rows *sql.Rows) ([]*models.Event, error) {
	items := make([]*models.Event, 0)
	for rows.Next() {
		item, err := scanEvent(rows)
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

func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
