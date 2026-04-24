package models

import "time"

type Event struct {
	ID                  int64      `json:"id"                              gorm:"column:id;primaryKey;autoIncrement"`
	Slug                string     `json:"slug"                            gorm:"column:slug;type:varchar(100);not null;uniqueIndex:uniq_events_slug"`
	Title               string     `json:"title"                           gorm:"column:title;type:varchar(255);not null"`
	Status              string     `json:"status"                          gorm:"column:status;type:varchar(20);not null;default:'draft';index:idx_events_status"`
	IsCurrent           bool       `json:"isCurrent"                       gorm:"column:is_current;not null;default:false"`
	RegistrationOpenAt  *time.Time `json:"registrationOpenAt,omitempty"    gorm:"column:registration_open_at"`
	RegistrationCloseAt *time.Time `json:"registrationCloseAt,omitempty"   gorm:"column:registration_close_at"`
	CreatedAt           time.Time  `json:"createdAt"                       gorm:"column:created_at;not null"`
	UpdatedAt           time.Time  `json:"updatedAt"                       gorm:"column:updated_at;not null"`
}

func (Event) TableName() string {
	return "events"
}
