package models

import "time"

type User struct {
	UID          int       `json:"uid"          gorm:"column:uid;primaryKey;autoIncrement"`
	Username     string    `json:"username"     gorm:"column:username;type:varchar(50);not null;uniqueIndex:uniq_users_username"`
	Email        string    `json:"email"        gorm:"column:email;type:varchar(255);not null;uniqueIndex:uniq_users_email"`
	PasswordHash string    `json:"-"            gorm:"column:password_hash;type:varchar(255);not null"`
	AvatarURL    *string   `json:"avatarUrl,omitempty" gorm:"column:avatar_url;type:varchar(500)"`
	Bio          *string   `json:"bio,omitempty"       gorm:"column:bio;type:text"`
	Phone        *string   `json:"phone,omitempty"     gorm:"column:phone;type:varchar(32)"`
	IsAdmin      bool      `json:"isAdmin"      gorm:"column:is_admin;not null;default:false"`
	Role         string    `json:"role"         gorm:"column:role;type:varchar(20);not null;default:'visitor';index:idx_users_role"`
	BKBalance    float64   `json:"bkBalance"    gorm:"column:bk_balance;type:numeric(10,2);not null;default:0"`
	TeamID       *int      `json:"teamId,omitempty" gorm:"column:team_id;index:idx_users_team_id"`
	CreatedAt    time.Time `json:"createdAt"    gorm:"column:created_at;not null"`
	UpdatedAt    time.Time `json:"updatedAt"    gorm:"column:updated_at;not null"`
}

func (User) TableName() string {
	return "users"
}
