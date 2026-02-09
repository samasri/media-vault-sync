package domain

import "time"

type Object struct {
	UID        string
	ProviderID string
	DatabaseID string
	VideoUID   string
	StorageKey string
	SizeBytes  int64
	Checksum   string
	CreatedAt  time.Time
}
