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
	Get(id int64) (MemoryItem, error)
	Update(id int64, content string, updatedAt time.Time) (bool, error)
	Delete(id int64) (bool, error)
	Close() error
}

// MetadataStore extends Store with structured-memory operations introduced by
// the M1 milestone. Implementations of Store are not required to satisfy this
// interface; callers MUST type-assert and degrade gracefully when the backing
// store does not support metadata (e.g. lightweight in-memory test fakes).
type MetadataStore interface {
	SaveWithMeta(req SaveRequest) (int64, error)
}

// FilterStore extends Store with the structured read paths introduced by
// the M2 milestone (recall with metadata filters + recent-context view).
// Same defensive pattern as MetadataStore: callers MUST type-assert and
// fail fast rather than silently degrade — silent degradation would let
// filter-aware callers receive unfiltered rows and corrupt downstream
// invariants.
type FilterStore interface {
	RecallFiltered(opts RecallOptions) ([]MemoryItem, error)
	Context(n int, scope string) ([]MemoryItem, error)
}

// RecallOptions carries the optional filter dimensions accepted by the
// M2 recall surface. Zero-value fields are treated as "unbounded":
//   - Query: required, non-empty after trim (validated by Service).
//   - Type:  empty string = no type filter.
//   - Since/Until: zero time = no lower/upper bound on created_at.
//   - Limit: ≤0 defaults to 10 at the service layer.
type RecallOptions struct {
	Query string
	Type  string
	Since time.Time
	Until time.Time
	Limit int
}

// SaveRequest carries the optional metadata fields accepted by the M1 save
// surface. CreatedAt MUST be set by the caller; the service is responsible
// for stamping the current UTC time when invoked through the public API.
type SaveRequest struct {
	Content   string
	Title     string
	Type      string
	TopicKey  string
	Scope     string
	CreatedAt time.Time
}

// ErrNotFound is the stable sentinel returned when a note id does not exist.
var ErrNotFound = errors.New("note not found")

type Service struct {
	repo Store
}

type MemoryItem struct {
	ID        int64
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
	// M1 metadata fields. Empty string is the documented "unset" marker so
	// the schema additions stay backward-compatible with existing callers.
	Title    string
	Type     string
	TopicKey string
	Scope    string
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

// SaveWithMeta persists a note with optional structured metadata. Empty
// `Type` defaults to "manual" and empty `Scope` defaults to "project" per
// design.md §metadata-defaults — the spec calls these out explicitly so
// downstream filters can rely on the defaults being non-empty.
//
// If the underlying store does not implement MetadataStore, the call fails
// fast rather than silently dropping metadata: silent drops would let
// downstream consumers see partially-populated rows and corrupt the
// upsert-by-topic invariant.
func (s *Service) SaveWithMeta(req SaveRequest) (int64, error) {
	clean := strings.TrimSpace(req.Content)
	if clean == "" {
		return 0, errors.New("content is empty")
	}
	meta, ok := s.repo.(MetadataStore)
	if !ok {
		return 0, errors.New("store does not support metadata operations")
	}

	if strings.TrimSpace(req.Type) == "" {
		req.Type = "manual"
	}
	if strings.TrimSpace(req.Scope) == "" {
		req.Scope = "project"
	}
	req.Content = clean
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}
	return meta.SaveWithMeta(req)
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

// RecallWithOptions runs a recall with optional metadata + date-range
// filters. Validation mirrors Recall(): empty/whitespace-only queries
// are rejected before the store is touched. Limit ≤0 defaults to 10.
//
// If the underlying store does not implement FilterStore, the call
// fails fast with an explicit error rather than degrading to plain
// Recall — silent degradation would let filter-aware callers see
// unfiltered rows.
func (s *Service) RecallWithOptions(opts RecallOptions) ([]MemoryItem, error) {
	clean := strings.TrimSpace(opts.Query)
	if clean == "" {
		return nil, errors.New("query is empty")
	}
	filt, ok := s.repo.(FilterStore)
	if !ok {
		return nil, errors.New("store does not support filter operations")
	}
	opts.Query = clean
	opts.Type = strings.TrimSpace(opts.Type)
	if opts.Limit <= 0 {
		opts.Limit = 10
	}
	return filt.RecallFiltered(opts)
}

// Context returns the most recent N notes (newest-first), optionally
// scoped to a single Scope value. n ≤0 defaults to 10. Scope is
// trimmed; empty string disables the scope filter. Same defensive
// FilterStore type-assert as RecallWithOptions.
func (s *Service) Context(n int, scope string) ([]MemoryItem, error) {
	filt, ok := s.repo.(FilterStore)
	if !ok {
		return nil, errors.New("store does not support context operations")
	}
	if n <= 0 {
		n = 10
	}
	scope = strings.TrimSpace(scope)
	return filt.Context(n, scope)
}

func (s *Service) List(limit int) ([]MemoryItem, error) {
	if limit <= 0 {
		limit = 10
	}
	return s.repo.List(limit)
}

func (s *Service) Get(id int64) (MemoryItem, error) {
	if id <= 0 {
		return MemoryItem{}, errors.New("id must be positive")
	}
	return s.repo.Get(id)
}

func (s *Service) Update(id int64, content string) (bool, error) {
	if id <= 0 {
		return false, errors.New("id must be positive")
	}
	clean := strings.TrimSpace(content)
	if clean == "" {
		return false, errors.New("content is empty")
	}
	return s.repo.Update(id, clean, time.Now().UTC())
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
