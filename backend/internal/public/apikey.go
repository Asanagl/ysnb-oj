// Package public holds the API-key credential model shared between the
// handler (which mints/validates keys) and the store (which migrates the
// table). It lives outside internal/handler to avoid a store → handler
// import cycle.
package public

import "time"

// ApiKeyRecord is a machine credential for the public API. The raw key is
// shown exactly once at creation; only its sha256 lives in the database.
type ApiKeyRecord struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Name      string     `gorm:"size:100" json:"name"`
	KeyHash   string     `gorm:"size:64;uniqueIndex" json:"-"`
	Prefix    string     `gorm:"size:12" json:"prefix"` // first chars for UI identification
	Revoked   bool       `gorm:"index" json:"revoked"`
	CreatedBy uint       `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	LastUsed  *time.Time `json:"last_used"`
}

func (ApiKeyRecord) TableName() string { return "api_keys" }