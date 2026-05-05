package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Store interface {
	Init() error
	Save(content string, createdAt time.Time) (int64, error)
	Recall(query string, limit int) ([]MemoryItem, error)
	List(limit int) ([]MemoryItem, error)
	Delete(id int64) (bool, error)
	Close() error
}

type Service struct {
	repo Store
}

type MemoryItem struct {
	ID        int64
	Content   string
	CreatedAt time.Time
}

func NewService(repo Store) *Service {
	return &Service{repo: repo}
}

func (s *Service) Init() error {
	return s.repo.Init()
}

func (s *Service) Save(content string) (int64, error) {
	clean := strings.TrimSpace(content)
	if clean == "" {
		return 0, errors.New("content is empty")
	}
	return s.repo.Save(clean, time.Now().UTC())
}

func (s *Service) Recall(query string, limit int) ([]MemoryItem, error) {
	clean := strings.TrimSpace(query)
	if clean == "" {
		return nil, errors.New("query is empty")
	}
	if limit <= 0 {
		limit = 10
	}
	return s.repo.Recall(clean, limit)
}

func (s *Service) List(limit int) ([]MemoryItem, error) {
	if limit <= 0 {
		limit = 10
	}
	return s.repo.List(limit)
}

func (s *Service) Delete(id int64) (bool, error) {
	if id <= 0 {
		return false, errors.New("id must be positive")
	}
	return s.repo.Delete(id)
}

func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		if h := strings.TrimSpace(os.Getenv("HOME")); h != "" {
			home = h
		} else {
			home = "/tmp"
		}
	}
	dir := filepath.Join(home, ".nt-cli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "data.db"), nil
}
