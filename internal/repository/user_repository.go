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
)

type UserRepository struct {
	db *sql.DB
}

type UpdateUserProfileParams struct {
	UID       int
	AvatarURL *string
	Bio       *string
	Phone     *string
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetByID(ctx context.Context, uid int) (*models.User, error) {
	row := r.db.QueryRowContext(ctx, userSelectByClause(`uid = $1`), uid)
	return scanUser(row)
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
	return r.exists(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE username = $1)`, strings.TrimSpace(username))
}

func (r *UserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return r.exists(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE email = $1)`, strings.TrimSpace(strings.ToLower(email)))
}

func (r *UserRepository) ExistsByUID(ctx context.Context, uid int) (bool, error) {
	return r.exists(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE uid = $1)`, uid)
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	_, err := r.db.ExecContext(
		ctx,
		`
		INSERT INTO users (
			uid, username, email, password_hash, avatar_url, bio, phone,
			is_admin, role, bk_balance, team_id, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13
		)
		`,
		user.UID,
		user.Username,
		user.Email,
		user.PasswordHash,
		user.AvatarURL,
		user.Bio,
		user.Phone,
		user.IsAdmin,
		user.Role,
		user.BKBalance,
		user.TeamID,
		user.CreatedAt,
		user.UpdatedAt,
	)
	return err
}

func (r *UserRepository) UpdateProfile(ctx context.Context, params UpdateUserProfileParams) (*models.User, error) {
	now := time.Now().UTC()

	if _, err := r.db.ExecContext(
		ctx,
		`
		UPDATE users
		SET avatar_url = $1,
			bio = $2,
			phone = $3,
			updated_at = $4
		WHERE uid = $5
		`,
		params.AvatarURL,
		params.Bio,
		params.Phone,
		now,
		params.UID,
	); err != nil {
		return nil, err
	}

	return r.GetByID(ctx, params.UID)
}

func (r *UserRepository) UpdateRole(ctx context.Context, uid int, role string) error {
	_, err := r.db.ExecContext(
		ctx,
		`
		UPDATE users
		SET role = $1,
			updated_at = $2
		WHERE uid = $3
		  AND is_admin = false
		`,
		role,
		time.Now().UTC(),
		uid,
	)
	return err
}

func (r *UserRepository) UpdatePassword(ctx context.Context, uid int, passwordHash string) error {
	_, err := r.db.ExecContext(
		ctx,
		`
		UPDATE users
		SET password_hash = $1,
			updated_at = $2
		WHERE uid = $3
		`,
		passwordHash,
		time.Now().UTC(),
		uid,
	)
	return err
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

func (r *UserRepository) exists(ctx context.Context, query string, arg any) (bool, error) {
	var exists bool
	if err := r.db.QueryRowContext(ctx, query, arg).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *UserRepository) getByUsername(ctx context.Context, username string) (*models.User, error) {
	row := r.db.QueryRowContext(ctx, userSelectByClause(`username = $1`), username)
	return scanUser(row)
}

func (r *UserRepository) getByEmail(ctx context.Context, email string) (*models.User, error) {
	row := r.db.QueryRowContext(ctx, userSelectByClause(`email = $1`), email)
	return scanUser(row)
}

func userSelectByClause(where string) string {
	return `
		SELECT
			uid, username, email, password_hash, avatar_url, bio, phone,
			is_admin, role, bk_balance, team_id, created_at, updated_at
		FROM users
		WHERE ` + where + `
		LIMIT 1
	`
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(scanner rowScanner) (*models.User, error) {
	var user models.User
	var avatarURL sql.NullString
	var bio sql.NullString
	var phone sql.NullString
	var teamID sql.NullInt64

	if err := scanner.Scan(
		&user.UID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&avatarURL,
		&bio,
		&phone,
		&user.IsAdmin,
		&user.Role,
		&user.BKBalance,
		&teamID,
		&user.CreatedAt,
		&user.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if avatarURL.Valid {
		user.AvatarURL = &avatarURL.String
	}
	if bio.Valid {
		user.Bio = &bio.String
	}
	if phone.Valid {
		user.Phone = &phone.String
	}
	if teamID.Valid {
		value := int(teamID.Int64)
		user.TeamID = &value
	}

	return &user, nil
}
