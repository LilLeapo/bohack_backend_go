package models

import "time"

type Registration struct {
	ID             int64      `json:"id"             gorm:"column:id;primaryKey;autoIncrement"`
	EventID        int64      `json:"eventId"        gorm:"column:event_id;not null;uniqueIndex:uniq_registration_event_user,priority:1;index:idx_event_registrations_event_status,priority:1"`
	EventSlug      string     `json:"eventSlug"      gorm:"-"`
	EventTitle     string     `json:"eventTitle"     gorm:"-"`
	UserID         int        `json:"userId"         gorm:"column:user_id;not null;uniqueIndex:uniq_registration_event_user,priority:2;index:idx_event_registrations_user_id"`
	Username       *string    `json:"username,omitempty" gorm:"-"`
	Status         string     `json:"status"         gorm:"column:status;type:varchar(20);not null;default:'submitted';index:idx_event_registrations_event_status,priority:2"`
	RealName       string     `json:"realName"       gorm:"column:real_name;type:varchar(100);not null"`
	Phone          string     `json:"phone"          gorm:"column:phone;type:varchar(32);not null"`
	EmailSnapshot  string     `json:"emailSnapshot"  gorm:"column:email_snapshot;type:varchar(255);not null"`
	School         *string    `json:"school,omitempty"         gorm:"column:school;type:varchar(255)"`
	Company        *string    `json:"company,omitempty"        gorm:"column:company;type:varchar(255)"`
	Bio            *string    `json:"bio,omitempty"            gorm:"column:bio;type:text"`
	TeamName       *string    `json:"teamName,omitempty"       gorm:"column:team_name;type:varchar(255)"`
	RolePreference *string    `json:"rolePreference,omitempty" gorm:"column:role_preference;type:varchar(50)"`
	Source         *string    `json:"source,omitempty"         gorm:"column:source;type:varchar(100)"`
	Note           *string    `json:"note,omitempty"           gorm:"column:note;type:text"`
	Extra          JSONB      `json:"extra"          gorm:"column:extra;type:text;not null;default:'{}'"`
	SubmittedAt    time.Time  `json:"submittedAt"    gorm:"column:submitted_at;not null"`
	ReviewedAt     *time.Time `json:"reviewedAt,omitempty"     gorm:"column:reviewed_at"`
	ReviewedBy     *int       `json:"reviewedBy,omitempty"     gorm:"column:reviewed_by"`
	ReviewNote     *string    `json:"reviewNote,omitempty"     gorm:"column:review_note;type:text"`
	CreatedAt      time.Time  `json:"createdAt"      gorm:"column:created_at;not null"`
	UpdatedAt      time.Time  `json:"updatedAt"      gorm:"column:updated_at;not null"`
}

func (Registration) TableName() string {
	return "event_registrations"
}
