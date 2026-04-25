package repository

import (
	"context"
	"encoding/json"
	"time"

	"bohack_backend_go/internal/models"

	"gorm.io/gorm"
)

type RegistrationRepository struct {
	db *gorm.DB
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

type AdminUpdateRegistrationParams struct {
	ID             int64
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

type registrationRow struct {
	ID             int64        `gorm:"column:id"`
	EventID        int64        `gorm:"column:event_id"`
	EventSlug      string       `gorm:"column:event_slug"`
	EventTitle     string       `gorm:"column:event_title"`
	UserID         int          `gorm:"column:user_id"`
	Username       *string      `gorm:"column:username"`
	Status         string       `gorm:"column:status"`
	RealName       string       `gorm:"column:real_name"`
	Phone          string       `gorm:"column:phone"`
	EmailSnapshot  string       `gorm:"column:email_snapshot"`
	School         *string      `gorm:"column:school"`
	Company        *string      `gorm:"column:company"`
	Bio            *string      `gorm:"column:bio"`
	TeamName       *string      `gorm:"column:team_name"`
	RolePreference *string      `gorm:"column:role_preference"`
	Source         *string      `gorm:"column:source"`
	Note           *string      `gorm:"column:note"`
	Extra          models.JSONB `gorm:"column:extra"`
	SubmittedAt    time.Time    `gorm:"column:submitted_at"`
	ReviewedAt     *time.Time   `gorm:"column:reviewed_at"`
	ReviewedBy     *int         `gorm:"column:reviewed_by"`
	ReviewNote     *string      `gorm:"column:review_note"`
	CreatedAt      time.Time    `gorm:"column:created_at"`
	UpdatedAt      time.Time    `gorm:"column:updated_at"`
}

func NewRegistrationRepository(db *gorm.DB) *RegistrationRepository {
	return &RegistrationRepository{db: db}
}

func (r *RegistrationRepository) GetByID(ctx context.Context, id int64) (*models.Registration, error) {
	rows, err := r.queryRows(ctx, "er.id = ?", []any{id}, "", 1, 0)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sqlNoRows()
	}
	item := rowToRegistration(rows[0])
	return &item, nil
}

func (r *RegistrationRepository) GetByUserAndEvent(ctx context.Context, userID int, eventID int64) (*models.Registration, error) {
	rows, err := r.queryRows(ctx, "er.user_id = ? AND er.event_id = ?", []any{userID, eventID}, "", 1, 0)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sqlNoRows()
	}
	item := rowToRegistration(rows[0])
	return &item, nil
}

func (r *RegistrationRepository) Create(ctx context.Context, params CreateRegistrationParams) (*models.Registration, error) {
	now := time.Now().UTC()
	extra := normalizeExtra(params.Extra)

	registration := models.Registration{
		EventID:        params.EventID,
		UserID:         params.UserID,
		Status:         params.Status,
		RealName:       params.RealName,
		Phone:          params.Phone,
		EmailSnapshot:  params.EmailSnapshot,
		School:         params.School,
		Company:        params.Company,
		Bio:            params.Bio,
		TeamName:       params.TeamName,
		RolePreference: params.RolePreference,
		Source:         params.Source,
		Note:           params.Note,
		Extra:          extra,
		SubmittedAt:    now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := r.db.WithContext(ctx).Create(&registration).Error; err != nil {
		return nil, err
	}

	return r.GetByID(ctx, registration.ID)
}

func (r *RegistrationRepository) UpdateSubmission(ctx context.Context, params UpdateRegistrationSubmissionParams) (*models.Registration, error) {
	now := time.Now().UTC()
	extra := normalizeExtra(params.Extra)

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Registration{}).
			Where("id = ?", params.ID).
			Updates(map[string]any{
				"status":          "submitted",
				"real_name":       params.RealName,
				"phone":           params.Phone,
				"email_snapshot":  params.EmailSnapshot,
				"school":          params.School,
				"company":         params.Company,
				"bio":             params.Bio,
				"team_name":       params.TeamName,
				"role_preference": params.RolePreference,
				"source":          params.Source,
				"note":            params.Note,
				"extra":           extra,
				"submitted_at":    now,
				"reviewed_at":     nil,
				"reviewed_by":     nil,
				"review_note":     nil,
				"updated_at":      now,
			}).Error; err != nil {
			return err
		}

		return tx.Model(&models.User{}).
			Where("uid = ? AND is_admin = ?", params.UserID, false).
			Updates(map[string]any{
				"role":       "visitor",
				"updated_at": now,
			}).Error
	})
	if err != nil {
		return nil, err
	}

	return r.GetByID(ctx, params.ID)
}

func (r *RegistrationRepository) AdminUpdate(ctx context.Context, params AdminUpdateRegistrationParams) (*models.Registration, error) {
	now := time.Now().UTC()
	extra := normalizeExtra(params.Extra)

	if err := r.db.WithContext(ctx).Model(&models.Registration{}).
		Where("id = ?", params.ID).
		Updates(map[string]any{
			"real_name":       params.RealName,
			"phone":           params.Phone,
			"email_snapshot":  params.EmailSnapshot,
			"school":          params.School,
			"company":         params.Company,
			"bio":             params.Bio,
			"team_name":       params.TeamName,
			"role_preference": params.RolePreference,
			"source":          params.Source,
			"note":            params.Note,
			"extra":           extra,
			"updated_at":      now,
		}).Error; err != nil {
		return nil, err
	}

	return r.GetByID(ctx, params.ID)
}

func (r *RegistrationRepository) Cancel(ctx context.Context, id int64, userID int) (*models.Registration, error) {
	now := time.Now().UTC()

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Registration{}).
			Where("id = ?", id).
			Updates(map[string]any{
				"status":      "cancelled",
				"reviewed_at": nil,
				"reviewed_by": nil,
				"review_note": nil,
				"updated_at":  now,
			}).Error; err != nil {
			return err
		}

		return tx.Model(&models.User{}).
			Where("uid = ? AND is_admin = ?", userID, false).
			Updates(map[string]any{
				"role":       "visitor",
				"updated_at": now,
			}).Error
	})
	if err != nil {
		return nil, err
	}

	return r.GetByID(ctx, id)
}

func (r *RegistrationRepository) Review(ctx context.Context, params ReviewRegistrationParams) (*models.Registration, error) {
	now := time.Now().UTC()

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if params.Status == "submitted" {
			if err := tx.Model(&models.Registration{}).
				Where("id = ?", params.ID).
				Updates(map[string]any{
					"status":      params.Status,
					"reviewed_at": nil,
					"reviewed_by": nil,
					"review_note": nil,
					"updated_at":  now,
				}).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&models.Registration{}).
				Where("id = ?", params.ID).
				Updates(map[string]any{
					"status":      params.Status,
					"reviewed_at": now,
					"reviewed_by": params.ReviewedBy,
					"review_note": params.ReviewNote,
					"updated_at":  now,
				}).Error; err != nil {
				return err
			}
		}

		userRole := roleForRegistrationStatus(params.Status)
		return tx.Model(&models.User{}).
			Where("uid = ? AND is_admin = ?", params.UserID, false).
			Updates(map[string]any{
				"role":       userRole,
				"updated_at": now,
			}).Error
	})
	if err != nil {
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

	whereSQL := "1 = 1"
	args := make([]any, 0, 4)
	if params.EventID != nil {
		whereSQL += " AND er.event_id = ?"
		args = append(args, *params.EventID)
	}
	if params.Status != "" {
		whereSQL += " AND er.status = ?"
		args = append(args, params.Status)
	}

	var total int64
	countQuery := "SELECT COUNT(1) FROM event_registrations er WHERE " + whereSQL
	if err := r.db.WithContext(ctx).Raw(countQuery, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.PageSize
	rows, err := r.queryRows(ctx, whereSQL, args, "er.created_at DESC, er.id DESC", params.PageSize, offset)
	if err != nil {
		return nil, 0, err
	}

	items := make([]*models.Registration, 0, len(rows))
	for _, row := range rows {
		item := rowToRegistration(row)
		items = append(items, &item)
	}

	return items, int(total), nil
}

func (r *RegistrationRepository) queryRows(ctx context.Context, where string, args []any, orderBy string, limit, offset int) ([]registrationRow, error) {
	tx := r.db.WithContext(ctx).
		Table("event_registrations er").
		Select(`
			er.id,
			er.event_id,
			e.slug AS event_slug,
			e.title AS event_title,
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
		`).
		Joins("INNER JOIN events e ON e.id = er.event_id").
		Joins("LEFT JOIN users u ON u.uid = er.user_id")

	if where != "" {
		tx = tx.Where(where, args...)
	}
	if orderBy != "" {
		tx = tx.Order(orderBy)
	}
	if limit > 0 {
		tx = tx.Limit(limit)
	}
	if offset > 0 {
		tx = tx.Offset(offset)
	}

	var rows []registrationRow
	if err := tx.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func rowToRegistration(row registrationRow) models.Registration {
	extra := row.Extra
	if len(extra) == 0 {
		extra = models.JSONB(`{}`)
	}
	return models.Registration{
		ID:             row.ID,
		EventID:        row.EventID,
		EventSlug:      row.EventSlug,
		EventTitle:     row.EventTitle,
		UserID:         row.UserID,
		Username:       row.Username,
		Status:         row.Status,
		RealName:       row.RealName,
		Phone:          row.Phone,
		EmailSnapshot:  row.EmailSnapshot,
		School:         row.School,
		Company:        row.Company,
		Bio:            row.Bio,
		TeamName:       row.TeamName,
		RolePreference: row.RolePreference,
		Source:         row.Source,
		Note:           row.Note,
		Extra:          extra,
		SubmittedAt:    row.SubmittedAt,
		ReviewedAt:     row.ReviewedAt,
		ReviewedBy:     row.ReviewedBy,
		ReviewNote:     row.ReviewNote,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func normalizeExtra(raw json.RawMessage) models.JSONB {
	if len(raw) == 0 || string(raw) == "null" {
		return models.JSONB(`{}`)
	}
	return models.JSONB(raw)
}

func sqlNoRows() error {
	return translateError(gorm.ErrRecordNotFound)
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
