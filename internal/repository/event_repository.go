package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"bohack_backend_go/internal/models"

	"gorm.io/gorm"
)

type EventRepository struct {
	db *gorm.DB
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

func NewEventRepository(db *gorm.DB) *EventRepository {
	return &EventRepository{db: db}
}

func (r *EventRepository) GetByID(ctx context.Context, id int64) (*models.Event, error) {
	var event models.Event
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&event).Error; err != nil {
		return nil, translateError(err)
	}
	return &event, nil
}

func (r *EventRepository) GetBySlug(ctx context.Context, slug string) (*models.Event, error) {
	var event models.Event
	if err := r.db.WithContext(ctx).Where("slug = ?", slug).Take(&event).Error; err != nil {
		return nil, translateError(err)
	}
	return &event, nil
}

func (r *EventRepository) GetPublicBySlug(ctx context.Context, slug string) (*models.Event, error) {
	var event models.Event
	if err := r.db.WithContext(ctx).
		Where("slug = ? AND status = ?", slug, "published").
		Take(&event).Error; err != nil {
		return nil, translateError(err)
	}
	return &event, nil
}

func (r *EventRepository) GetCurrent(ctx context.Context, fallbackSlug string) (*models.Event, error) {
	var event models.Event
	err := r.db.WithContext(ctx).
		Where("is_current = ? AND status = ?", true, "published").
		Take(&event).Error
	if err == nil {
		return &event, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if strings.TrimSpace(fallbackSlug) != "" {
		fallback, fallbackErr := r.GetPublicBySlug(ctx, fallbackSlug)
		if fallbackErr == nil {
			return fallback, nil
		}
		if !errors.Is(fallbackErr, sql.ErrNoRows) {
			return nil, fallbackErr
		}
	}

	if err := r.db.WithContext(ctx).
		Where("status = ?", "published").
		Order("created_at DESC, id DESC").
		Take(&event).Error; err != nil {
		return nil, translateError(err)
	}
	return &event, nil
}

func (r *EventRepository) ListPublic(ctx context.Context) ([]*models.Event, error) {
	var events []*models.Event
	if err := r.db.WithContext(ctx).
		Where("status = ?", "published").
		Order("is_current DESC, created_at DESC, id DESC").
		Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

func (r *EventRepository) List(ctx context.Context, status string) ([]*models.Event, error) {
	var events []*models.Event
	tx := r.db.WithContext(ctx)
	if status != "" {
		tx = tx.Where("status = ?", status)
	}
	if err := tx.Order("is_current DESC, created_at DESC, id DESC").Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

func (r *EventRepository) Create(ctx context.Context, params CreateEventParams) (*models.Event, error) {
	now := time.Now().UTC()
	event := models.Event{
		Slug:                params.Slug,
		Title:               params.Title,
		Status:              params.Status,
		IsCurrent:           params.IsCurrent,
		RegistrationOpenAt:  params.RegistrationOpenAt,
		RegistrationCloseAt: params.RegistrationCloseAt,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if params.IsCurrent {
			if err := tx.Model(&models.Event{}).
				Where("is_current = ?", true).
				Update("is_current", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&event).Error
	})
	if err != nil {
		return nil, err
	}

	return r.GetByID(ctx, event.ID)
}

func (r *EventRepository) Update(ctx context.Context, params UpdateEventParams) (*models.Event, error) {
	now := time.Now().UTC()

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if params.IsCurrent {
			if err := tx.Model(&models.Event{}).
				Where("is_current = ? AND id <> ?", true, params.ID).
				Update("is_current", false).Error; err != nil {
				return err
			}
		}
		return tx.Model(&models.Event{}).
			Where("id = ?", params.ID).
			Updates(map[string]any{
				"slug":                  params.Slug,
				"title":                 params.Title,
				"status":                params.Status,
				"is_current":            params.IsCurrent,
				"registration_open_at":  params.RegistrationOpenAt,
				"registration_close_at": params.RegistrationCloseAt,
				"updated_at":            now,
			}).Error
	})
	if err != nil {
		return nil, err
	}

	return r.GetByID(ctx, params.ID)
}
