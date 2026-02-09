package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type StagingStorage struct {
	basePath string
}

func NewStagingStorage(basePath string) *StagingStorage {
	return &StagingStorage{basePath: basePath}
}

func (s *StagingStorage) Store(ctx context.Context, key string, data []byte) error {
	path := filepath.Join(s.basePath, key)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating staging directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing staging file: %w", err)
	}
	return nil
}

func (s *StagingStorage) Load(ctx context.Context, key string) ([]byte, error) {
	path := filepath.Join(s.basePath, key)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading staging file: %w", err)
	}
	return data, nil
}

func (s *StagingStorage) Delete(ctx context.Context, key string) error {
	path := filepath.Join(s.basePath, key)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting staging file: %w", err)
	}
	return nil
}
