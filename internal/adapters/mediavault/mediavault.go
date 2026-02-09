package mediavault

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
)

type VideoSender interface {
	SendVideo(ctx context.Context, databaseID, albumUID, videoUID string, data []byte) error
}

type DatabaseScopedMediaVault struct {
	configPath  string
	databaseID  string
	videoSender VideoSender
}

func NewDatabaseScopedMediaVault(configPath, databaseID string, sender VideoSender) *DatabaseScopedMediaVault {
	return &DatabaseScopedMediaVault{
		configPath:  configPath,
		databaseID:  databaseID,
		videoSender: sender,
	}
}

func (p *DatabaseScopedMediaVault) readConfig() (*Config, error) {
	data, err := os.ReadFile(p.configPath)
	if err != nil {
		return nil, fmt.Errorf("reading mediavault config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing mediavault config: %w", err)
	}
	return &cfg, nil
}

func (p *DatabaseScopedMediaVault) findUser(cfg *Config, userID string) *UserConfig {
	for _, prov := range cfg.Providers {
		for _, db := range prov.Databases {
			if db.DatabaseID != p.databaseID {
				continue
			}
			for i := range db.Users {
				if db.Users[i].UserID == userID {
					return &db.Users[i]
				}
			}
		}
	}
	return nil
}

func (p *DatabaseScopedMediaVault) findAlbum(cfg *Config, albumUID string) (*AlbumConfig, *UserConfig) {
	for _, prov := range cfg.Providers {
		for _, db := range prov.Databases {
			if db.DatabaseID != p.databaseID {
				continue
			}
			for i := range db.Users {
				for j := range db.Users[i].Albums {
					if db.Users[i].Albums[j].AlbumUID == albumUID {
						return &db.Users[i].Albums[j], &db.Users[i]
					}
				}
			}
		}
	}
	return nil, nil
}

func (p *DatabaseScopedMediaVault) ListAlbumUIDs(ctx context.Context, userID string) ([]string, error) {
	cfg, err := p.readConfig()
	if err != nil {
		return nil, err
	}

	user := p.findUser(cfg, userID)
	if user == nil {
		return nil, nil
	}

	var albumUIDs []string
	for _, album := range user.Albums {
		albumUIDs = append(albumUIDs, album.AlbumUID)
	}
	return albumUIDs, nil
}

func (p *DatabaseScopedMediaVault) ListVideoUIDs(ctx context.Context, albumUID string) ([]string, error) {
	cfg, err := p.readConfig()
	if err != nil {
		return nil, err
	}

	album, _ := p.findAlbum(cfg, albumUID)
	if album == nil {
		return nil, nil
	}

	return album.Videos, nil
}

func (p *DatabaseScopedMediaVault) GetUserIDForAlbum(ctx context.Context, albumUID string) (string, error) {
	cfg, err := p.readConfig()
	if err != nil {
		return "", err
	}

	_, user := p.findAlbum(cfg, albumUID)
	if user == nil {
		return "", nil
	}

	return user.UserID, nil
}

func (p *DatabaseScopedMediaVault) CMove(ctx context.Context, albumUID string) error {
	cfg, err := p.readConfig()
	if err != nil {
		return err
	}

	album, _ := p.findAlbum(cfg, albumUID)
	if album == nil {
		return nil
	}

	for _, videoUID := range album.Videos {
		data := make([]byte, 2*1024*1024) // 2MB
		rand.Read(data)
		if err := p.videoSender.SendVideo(ctx, p.databaseID, albumUID, videoUID, data); err != nil {
			return fmt.Errorf("sending video %s: %w", videoUID, err)
		}
	}

	return nil
}
