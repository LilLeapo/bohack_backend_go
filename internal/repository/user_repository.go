package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"math/big"
	"strings"
	"time"

	"bohack_backend_go/internal/models"

	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

type UpdateUserProfileParams struct {
	UID       int
	AvatarURL *string
	Bio       *string
	Phone     *string
}

type ListUsersParams struct {
	Query    string
	Role     string
	Page     int
	PageSize int
}

type AdminUpdateUserParams struct {
	UID       int
	Username  string
	Email     string
	AvatarURL *string
	Bio       *string
	Phone     *string
	IsAdmin   bool
	Role      string
	BKBalance float64
	TeamID    *int
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetByID(ctx context.Context, uid int) (*models.User, error) {
	var user models.User
	if err := r.db.WithContext(ctx).Where("uid = ?", uid).Take(&user).Error; err != nil {
		return nil, translateError(err)
	}
	return &user, nil
}

func (r *UserRepository) GetByLogin(ctx context.Context, username, email string) (*models.User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(strings.ToLower(email))

	switch {
	case username != "" && email != "":
		userByUsername, err := r.getByUsername(ctx, username)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}

		userByEmail, err := r.getByEmail(ctx, email)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}

		if userByUsername == nil || userByEmail == nil {
			return nil, sql.ErrNoRows
		}
		if userByUsername.UID != userByEmail.UID {
			return nil, sql.ErrNoRows
		}
		return userByUsername, nil
	case username != "":
		return r.getByUsername(ctx, username)
	case email != "":
		return r.getByEmail(ctx, email)
	default:
		return nil, sql.ErrNoRows
	}
}

func (r *UserRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	return r.exists(ctx, "username = ?", strings.TrimSpace(username))
}

func (r *UserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return r.exists(ctx, "email = ?", strings.TrimSpace(strings.ToLower(email)))
}

func (r *UserRepository) ExistsByUID(ctx context.Context, uid int) (bool, error) {
	return r.exists(ctx, "uid = ?", uid)
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *UserRepository) UpdateProfile(ctx context.Context, params UpdateUserProfileParams) (*models.User, error) {
	now := time.Now().UTC()

	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("uid = ?", params.UID).
		Updates(map[string]any{
			"avatar_url": params.AvatarURL,
			"bio":        params.Bio,
			"phone":      params.Phone,
			"updated_at": now,
		}).Error; err != nil {
		return nil, err
	}

	return r.GetByID(ctx, params.UID)
}

func (r *UserRepository) List(ctx context.Context, params ListUsersParams) ([]*models.User, int, error) {
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}

	tx := r.db.WithContext(ctx).Model(&models.User{})
	if params.Role != "" {
		tx = tx.Where("role = ?", params.Role)
	}
	if params.Query != "" {
		like := "%" + strings.ToLower(strings.TrimSpace(params.Query)) + "%"
		tx = tx.Where("LOWER(username) LIKE ? OR LOWER(email) LIKE ? OR phone LIKE ?", like, like, "%"+params.Query+"%")
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []*models.User
	offset := (params.Page - 1) * params.PageSize
	if err := tx.Order("created_at DESC, uid DESC").
		Limit(params.PageSize).
		Offset(offset).
		Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, int(total), nil
}

func (r *UserRepository) AdminUpdate(ctx context.Context, params AdminUpdateUserParams) (*models.User, error) {
	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("uid = ?", params.UID).
		Updates(map[string]any{
			"username":   params.Username,
			"email":      params.Email,
			"avatar_url": params.AvatarURL,
			"bio":        params.Bio,
			"phone":      params.Phone,
			"is_admin":   params.IsAdmin,
			"role":       params.Role,
			"bk_balance": params.BKBalance,
			"team_id":    params.TeamID,
			"updated_at": time.Now().UTC(),
		}).Error; err != nil {
		return nil, err
	}

	return r.GetByID(ctx, params.UID)
}

func (r *UserRepository) UpdateRole(ctx context.Context, uid int, role string) error {
	return r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("uid = ? AND is_admin = ?", uid, false).
		Updates(map[string]any{
			"role":       role,
			"updated_at": time.Now().UTC(),
		}).Error
}

func (r *UserRepository) UpdatePassword(ctx context.Context, uid int, passwordHash string) error {
	return r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("uid = ?", uid).
		Updates(map[string]any{
			"password_hash": passwordHash,
			"updated_at":    time.Now().UTC(),
		}).Error
}

func (r *UserRepository) GenerateUID(ctx context.Context) (int, error) {
	for attempt := 0; attempt < 64; attempt++ {
		value, err := rand.Int(rand.Reader, big.NewInt(899999))
		if err != nil {
			return 0, err
		}

		uid := int(value.Int64()) + 100001
		exists, err := r.ExistsByUID(ctx, uid)
		if err != nil {
			return 0, err
		}
		if !exists {
			return uid, nil
		}
	}
	return 0, errors.New("unable to generate unique uid")
}

func (r *UserRepository) exists(ctx context.Context, where string, arg any) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where(where, arg).
		Limit(1).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *UserRepository) getByUsername(ctx context.Context, username string) (*models.User, error) {
	var user models.User
	if err := r.db.WithContext(ctx).Where("username = ?", username).Take(&user).Error; err != nil {
		return nil, translateError(err)
	}
	return &user, nil
}

func (r *UserRepository) getByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	if err := r.db.WithContext(ctx).Where("email = ?", email).Take(&user).Error; err != nil {
		return nil, translateError(err)
	}
	return &user, nil
}
