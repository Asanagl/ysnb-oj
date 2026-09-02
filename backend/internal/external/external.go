// Package external holds the practice-sync persistence models. why a
// separate package: store migrates them, but the sync worker and report
// handlers live in internal/handler, and store→handler would be an import
// cycle (same pattern as internal/public's ApiKeyRecord).
package external

import "time"

// Record is one fetched submission row from an external platform, stored
// raw enough to rebuild any report without re-crawling.
type Record struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// UserID+Platform index the per-user report; ExternalID is the global
	// dedup key (platform prefix + platform-native submission id).
	UserID     uint   `gorm:"index:idx_ext_user_platform" json:"user_id"`
	Platform   string `gorm:"size:32;index:idx_ext_user_platform" json:"platform"`
	ExternalID string `gorm:"uniqueIndex" json:"external_id"`
	ProblemID  string `gorm:"size:64" json:"problem_id"`
	ProblemName string `gorm:"size:200" json:"problem_name"`
	Verdict    string `gorm:"size:16" json:"verdict"`
	Language   string `gorm:"size:64" json:"language"`
	// At is the platform-side submit time (unix seconds).
	At        int64     `json:"at"`
	FetchedAt time.Time `json:"fetched_at"`
}

func (Record) TableName() string { return "external_records" }

// Binding is a user's handle on one external platform (e.g. Codeforces id).
type Binding struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	UserID   uint   `gorm:"uniqueIndex:idx_bind_user_platform" json:"user_id"`
	Platform string `gorm:"size:32;uniqueIndex:idx_bind_user_platform" json:"platform"`
	Handle   string `gorm:"size:128" json:"handle"`
	// SyncedAt is the last successful incremental sync (nil = never).
	SyncedAt  *time.Time `json:"synced_at"`
	LastError string     `gorm:"size:500" json:"last_error,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (Binding) TableName() string { return "external_bindings" }