package models

import (
	"encoding/json"
	"time"
)

type Registration struct {
	ID             int64           `json:"id"`
	EventID        int64           `json:"eventId"`
	EventSlug      string          `json:"eventSlug"`
	EventTitle     string          `json:"eventTitle"`
	UserID         int             `json:"userId"`
	Username       *string         `json:"username,omitempty"`
	Status         string          `json:"status"`
	RealName       string          `json:"realName"`
	Phone          string          `json:"phone"`
	EmailSnapshot  string          `json:"emailSnapshot"`
	School         *string         `json:"school,omitempty"`
	Company        *string         `json:"company,omitempty"`
	Bio            *string         `json:"bio,omitempty"`
	TeamName       *string         `json:"teamName,omitempty"`
	RolePreference *string         `json:"rolePreference,omitempty"`
	Source         *string         `json:"source,omitempty"`
	Note           *string         `json:"note,omitempty"`
	Extra          json.RawMessage `json:"extra"`
	SubmittedAt    time.Time       `json:"submittedAt"`
	ReviewedAt     *time.Time      `json:"reviewedAt,omitempty"`
	ReviewedBy     *int            `json:"reviewedBy,omitempty"`
	ReviewNote     *string         `json:"reviewNote,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}
