package app

import (
	"encoding/json"
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

// SessionEvent is a single row in the session-lifecycle log. Kind is one
// of "start", "summary", "end". Summary is non-empty only for "summary"
// rows. CreatedAt is UTC.
type SessionEvent struct {
	SessionID string
	Kind      string
	Summary   string
	CreatedAt time.Time
}

// SessionStore extends Store with the lifecycle log introduced by M3.
// Implementations of Store are NOT required to satisfy this interface;
// the service layer type-asserts and fails fast for legacy fakes (same
// pattern as MetadataStore / FilterStore).
type SessionStore interface {
	SessionStart(id string, at time.Time) error
	SessionSummary(id, summary string, at time.Time) error
	SessionEnd(id string, at time.Time) error
	SessionEvents(id string) ([]SessionEvent, error)
}

// ImportRecord is a single row queued for the M3 import bridge. Empty
// metadata fields default to type=manual / scope=project at the store
// layer (mirrors SaveWithMeta behaviour). CreatedAt zero = stamp now.
type ImportRecord struct {
	Content  string
	Title    string
	Type     string
	TopicKey string
	Scope    string
}

// ImportResult is the count summary returned by ImportRecords. Inserted +
// Skipped MUST equal len(input). Skipped covers both dedupe hits and
// validation drops (empty content) so callers can render a single
// "no-op" status from a single field.
type ImportResult struct {
	Inserted int
	Skipped  int
}

// ImportStore extends Store with the idempotent import path introduced
// by M3. Dedupe key is `(topic_key, sha256(content))`. Same defensive
// type-assert pattern as the other M3 capability interfaces.
type ImportStore interface {
	ImportRecords(rows []ImportRecord) (ImportResult, error)
}

// BackupStore extends Store with portable snapshot + restore for the
// M3 backup/restore feature. Implementations MUST produce a single-file
// artifact that can be moved across machines and restored losslessly.
// Defensive type-assert pattern: Service.Backup/Restore surface a clear
// capability error rather than a nil-method panic when the underlying
// store is a stub or doesn't support snapshots.
type BackupStore interface {
	Backup(dst string) error
	Restore(src string) error
}

// DoctorReport mirrors the store-layer diagnostic snapshot. Re-declared
// at the service layer (instead of re-exporting the store type) so the
// `app` package stays independent of `store` — keeping the same
// dependency direction the rest of the package follows.
type DoctorReport struct {
	SchemaVersion     int
	FTSHealthy        bool
	IntegrityOK       bool
	IntegrityMessages []string
	MemoryItemsCount  int
	SessionsCount     int
	Summary           string
}

// DoctorStore extends Store with the M3 diagnostic surface. Doctor
// MUST be read-only — callers (CLI, MCP) rely on this guarantee to run
// it without backup safeguards.
type DoctorStore interface {
	Doctor() (DoctorReport, error)
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

// SessionStart appends a "start" lifecycle row tagged with id. The
// service trims the id and stamps the current UTC time; empty/
// whitespace-only ids are rejected so unrelated sessions cannot be
// silently merged. Fails fast if the store does not implement
// SessionStore (same defensive pattern as MetadataStore/FilterStore).
func (s *Service) SessionStart(id string) error {
	clean, err := s.validateSessionID(id)
	if err != nil {
		return err
	}
	sess, ok := s.repo.(SessionStore)
	if !ok {
		return errors.New("store does not support session operations")
	}
	return sess.SessionStart(clean, time.Now().UTC())
}

// SessionEnd appends an "end" lifecycle row. Multiple ends are tolerated
// at the store layer — interpretation is the reader's responsibility.
func (s *Service) SessionEnd(id string) error {
	clean, err := s.validateSessionID(id)
	if err != nil {
		return err
	}
	sess, ok := s.repo.(SessionStore)
	if !ok {
		return errors.New("store does not support session operations")
	}
	return sess.SessionEnd(clean, time.Now().UTC())
}

// SessionSummary appends a "summary" lifecycle row. Empty/whitespace-only
// summaries are rejected — a summary row with no content has no value
// for downstream consumers and would just clutter the log.
func (s *Service) SessionSummary(id, summary string) error {
	clean, err := s.validateSessionID(id)
	if err != nil {
		return err
	}
	cleanSummary := strings.TrimSpace(summary)
	if cleanSummary == "" {
		return errors.New("summary is empty")
	}
	sess, ok := s.repo.(SessionStore)
	if !ok {
		return errors.New("store does not support session operations")
	}
	return sess.SessionSummary(clean, cleanSummary, time.Now().UTC())
}

// SessionEvents returns every lifecycle row tagged to id, in insertion
// order. Validation mirrors the write paths.
func (s *Service) SessionEvents(id string) ([]SessionEvent, error) {
	clean, err := s.validateSessionID(id)
	if err != nil {
		return nil, err
	}
	sess, ok := s.repo.(SessionStore)
	if !ok {
		return nil, errors.New("store does not support session operations")
	}
	return sess.SessionEvents(clean)
}

func (s *Service) validateSessionID(id string) (string, error) {
	clean := strings.TrimSpace(id)
	if clean == "" {
		return "", errors.New("session id is empty")
	}
	return clean, nil
}

// importJSONRecord mirrors ImportRecord with JSON tags. Kept private —
// callers send raw bytes and the service handles the parse so we can
// add MD/CSV later without exposing parser details.
type importJSONRecord struct {
	Content  string `json:"content"`
	Title    string `json:"title,omitempty"`
	Type     string `json:"type,omitempty"`
	TopicKey string `json:"topic_key,omitempty"`
	Scope    string `json:"scope,omitempty"`
}

// ImportJSON parses a JSON array of records and either writes them via
// ImportRecords (idempotent dedupe at the store) or, in dry-run mode,
// returns the planned insert count without touching the store.
//
// Empty/whitespace-only `content` is dropped client-side so partially
// malformed files still surface valid rows. The store does its own
// dedupe by `(topic_key, sha256(content))` — this method only handles
// parsing + dry-run accounting.
func (s *Service) ImportJSON(data []byte, dryRun bool) (ImportResult, error) {
	var records []importJSONRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return ImportResult{}, err
	}
	rows := make([]ImportRecord, 0, len(records))
	for _, r := range records {
		if strings.TrimSpace(r.Content) == "" {
			continue
		}
		rows = append(rows, ImportRecord{
			Content:  r.Content,
			Title:    r.Title,
			Type:     r.Type,
			TopicKey: r.TopicKey,
			Scope:    r.Scope,
		})
	}
	if dryRun {
		// In dry-run we report the local plan: every valid row is a
		// planned insert. The store's dedupe pass would refine this,
		// but we can't peek at the store without writing — that's the
		// honest contract per spec scenario "Dry-run reports without
		// writing".
		return ImportResult{Inserted: len(rows)}, nil
	}
	imp, ok := s.repo.(ImportStore)
	if !ok {
		return ImportResult{}, errors.New("store does not support import operations")
	}
	return imp.ImportRecords(rows)
}

// Backup creates a portable snapshot of the live store at dst. Returns
// a capability error if the underlying Store does not implement
// BackupStore (defensive type-assert mirrors session/import).
func (s *Service) Backup(dst string) error {
	clean := strings.TrimSpace(dst)
	if clean == "" {
		return errors.New("backup destination is empty")
	}
	bs, ok := s.repo.(BackupStore)
	if !ok {
		return errors.New("store does not support backup operations")
	}
	return bs.Backup(clean)
}

// Restore replaces the live store with the contents of src. Same
// capability check + trim semantics as Backup. Callers are expected to
// re-Init the service afterwards if the artifact predates the current
// schema version — Init is forward-only and idempotent.
func (s *Service) Restore(src string) error {
	clean := strings.TrimSpace(src)
	if clean == "" {
		return errors.New("restore source is empty")
	}
	bs, ok := s.repo.(BackupStore)
	if !ok {
		return errors.New("store does not support restore operations")
	}
	return bs.Restore(clean)
}

// Doctor returns the read-only diagnostic snapshot from the underlying
// store. Returns a capability error if the Store doesn't implement
// DoctorStore (defensive type-assert mirrors session/import/backup).
func (s *Service) Doctor() (DoctorReport, error) {
	ds, ok := s.repo.(DoctorStore)
	if !ok {
		return DoctorReport{}, errors.New("store does not support doctor diagnostics")
	}
	return ds.Doctor()
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
