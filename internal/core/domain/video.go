package domain

import "time"

type Video struct {
	UID        string
	ProviderID string
	DatabaseID string
	UserID     string
	VideoUID   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
