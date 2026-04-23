package models

import "time"

type Event struct {
	ID                  int64      `json:"id"`
	Slug                string     `json:"slug"`
	Title               string     `json:"title"`
	Status              string     `json:"status"`
	IsCurrent           bool       `json:"isCurrent"`
	RegistrationOpenAt  *time.Time `json:"registrationOpenAt,omitempty"`
	RegistrationCloseAt *time.Time `json:"registrationCloseAt,omitempty"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}
