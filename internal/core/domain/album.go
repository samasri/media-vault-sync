package domain

import "time"

type Album struct {
	UID        string
	ProviderID string
	DatabaseID string
	UserID     string
	AlbumUID   string
	Synced     bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
