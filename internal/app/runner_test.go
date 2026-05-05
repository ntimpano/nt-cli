package app_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"nt-cli/internal/app"
)

// memStore is an in-memory Store used to drive the CLI runner under test
// without touching SQLite. It is intentionally minimal — only the behaviours
// exercised by the runner tests are implemented faithfully.
type memStore struct {
	items  map[int64]app.MemoryItem
	nextID int64
}

func newMemStore() *memStore {
	return &memStore{items: map[int64]app.MemoryItem{}, nextID: 1}
}

func (m *memStore) Init() error { return nil }

func (m *memStore) Save(content string, createdAt time.Time) (int64, error) {
	id := m.nextID
	m.nextID++
	m.items[id] = app.MemoryItem{ID: id, Content: content, CreatedAt: createdAt, UpdatedAt: createdAt}
	return id, nil
}

func (m *memStore) Recall(query string, limit int) ([]app.MemoryItem, error) {
	var out []app.MemoryItem
	for _, it := range m.items {
		if strings.Contains(it.Content, query) {
			out = append(out, it)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *memStore) List(limit int) ([]app.MemoryItem, error) {
	out := make([]app.MemoryItem, 0, len(m.items))
	for _, it := range m.items {
		out = append(out, it)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *memStore) Get(id int64) (app.MemoryItem, error) {
	it, ok := m.items[id]
	if !ok {
		return app.MemoryItem{}, app.ErrNotFound
	}
	return it, nil
}

func (m *memStore) Update(id int64, content string, updatedAt time.Time) (bool, error) {
	it, ok := m.items[id]
	if !ok {
		return false, nil
	}
	it.Content = content
	it.UpdatedAt = updatedAt
	m.items[id] = it
	return true, nil
}

func (m *memStore) Delete(id int64) (bool, error) {
	if _, ok := m.items[id]; !ok {
		return false, nil
	}
	delete(m.items, id)
	return true, nil
}

func (m *memStore) Close() error { return nil }

func runCLI(t *testing.T, store *memStore, args ...string) (int, string, string) {
	t.Helper()
	svc := app.NewService(store)
	var stdout, stderr bytes.Buffer
	code := app.RunCLI(svc, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// --- get -------------------------------------------------------------------

func TestRunCLI_GetExistingIDPrintsNoteAndExitsZero(t *testing.T) {
	store := newMemStore()
	id, _ := store.Save("hello", time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))

	code, stdout, stderr := runCLI(t, store, "get", "1")

	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "#1") || !strings.Contains(stdout, "hello") {
		t.Fatalf("expected note rendered on stdout, got %q", stdout)
	}
	_ = id
}

func TestRunCLI_GetMissingIDExitsNonZero(t *testing.T) {
	store := newMemStore()

	code, stdout, stderr := runCLI(t, store, "get", "999")

	if code == 0 {
		t.Fatalf("expected non-zero exit on missing id, stdout=%q stderr=%q", stdout, stderr)
	}
	if stderr == "" {
		t.Fatalf("expected error message on stderr, got empty")
	}
}

func TestRunCLI_GetInvalidIDExitsNonZero(t *testing.T) {
	store := newMemStore()

	cases := []string{"abc", "0", "-3", ""}
	for _, raw := range cases {
		code, _, stderr := runCLI(t, store, "get", raw)
		if code == 0 {
			t.Fatalf("expected non-zero exit for raw=%q", raw)
		}
		if stderr == "" {
			t.Fatalf("expected stderr for raw=%q", raw)
		}
	}
}

func TestRunCLI_GetMissingArgExitsNonZero(t *testing.T) {
	store := newMemStore()

	code, _, stderr := runCLI(t, store, "get")
	if code == 0 {
		t.Fatalf("expected non-zero exit when id arg omitted")
	}
	if !strings.Contains(stderr, "usage") {
		t.Fatalf("expected usage hint on stderr, got %q", stderr)
	}
}

// --- update ----------------------------------------------------------------

func TestRunCLI_UpdateExistingIDChangesContent(t *testing.T) {
	store := newMemStore()
	store.Save("old", time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))

	code, stdout, stderr := runCLI(t, store, "update", "1", "new", "content")

	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "updated #1") {
		t.Fatalf("expected confirmation on stdout, got %q", stdout)
	}
	got, _ := store.Get(1)
	if got.Content != "new content" {
		t.Fatalf("expected store mutated to 'new content', got %q", got.Content)
	}
}

func TestRunCLI_UpdateMissingIDExitsNonZero(t *testing.T) {
	store := newMemStore()

	code, _, stderr := runCLI(t, store, "update", "999", "x")
	if code == 0 {
		t.Fatalf("expected non-zero exit on missing id update")
	}
	if !strings.Contains(stderr, "999") {
		t.Fatalf("expected id in stderr, got %q", stderr)
	}
}

func TestRunCLI_UpdateEmptyContentExitsNonZero(t *testing.T) {
	store := newMemStore()
	store.Save("old", time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))

	code, _, stderr := runCLI(t, store, "update", "1", "   ")
	if code == 0 {
		t.Fatalf("expected non-zero exit on empty content")
	}
	if stderr == "" {
		t.Fatalf("expected stderr message")
	}
	got, _ := store.Get(1)
	if got.Content != "old" {
		t.Fatalf("expected content unchanged, got %q", got.Content)
	}
}

func TestRunCLI_UpdateInvalidIDExitsNonZero(t *testing.T) {
	store := newMemStore()
	code, _, stderr := runCLI(t, store, "update", "abc", "x")
	if code == 0 {
		t.Fatalf("expected non-zero exit on invalid id")
	}
	if stderr == "" {
		t.Fatalf("expected stderr message")
	}
}

// --- delete (regression) ---------------------------------------------------

func TestRunCLI_DeleteExistingIDRemovesNote(t *testing.T) {
	store := newMemStore()
	store.Save("doomed", time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))

	code, stdout, stderr := runCLI(t, store, "delete", "1")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "deleted #1") {
		t.Fatalf("expected confirmation, got %q", stdout)
	}
	if _, err := store.Get(1); err == nil {
		t.Fatalf("expected note removed from store after delete")
	}
}

func TestRunCLI_DeleteMissingIDIsNotAnError(t *testing.T) {
	store := newMemStore()

	code, stdout, _ := runCLI(t, store, "delete", "999")
	// existing main.go behaviour: missing delete exits 0 and prints "not found"
	if code != 0 {
		t.Fatalf("expected exit 0 on missing delete (matches existing behaviour), got %d", code)
	}
	if !strings.Contains(stdout, "not found") {
		t.Fatalf("expected 'not found' on stdout, got %q", stdout)
	}
}

func TestRunCLI_DeleteInvalidIDExitsNonZero(t *testing.T) {
	store := newMemStore()
	code, _, stderr := runCLI(t, store, "delete", "abc")
	if code == 0 {
		t.Fatalf("expected non-zero exit on invalid delete id")
	}
	if stderr == "" {
		t.Fatalf("expected stderr message")
	}
}

// --- save / recall / list (regression for unchanged commands) -------------

func TestRunCLI_SaveAndListRegression(t *testing.T) {
	store := newMemStore()

	code, stdout, _ := runCLI(t, store, "save", "first", "note")
	if code != 0 {
		t.Fatalf("save failed: code=%d stdout=%q", code, stdout)
	}
	if !strings.Contains(stdout, "saved #1") {
		t.Fatalf("expected saved #1, got %q", stdout)
	}

	code, listOut, _ := runCLI(t, store, "list")
	if code != 0 {
		t.Fatalf("list failed: code=%d", code)
	}
	if !strings.Contains(listOut, "first note") {
		t.Fatalf("expected saved content in list output, got %q", listOut)
	}
}

func TestRunCLI_UnknownCommandExitsNonZero(t *testing.T) {
	store := newMemStore()
	code, stdout, stderr := runCLI(t, store, "frobnicate")
	if code == 0 {
		t.Fatalf("expected non-zero exit for unknown command, stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestRunCLI_NoArgsPrintsUsageAndExitsNonZero(t *testing.T) {
	store := newMemStore()
	code, stdout, _ := runCLI(t, store, []string{}...)
	if code == 0 {
		t.Fatalf("expected non-zero exit when called with no args")
	}
	if !strings.Contains(stdout, "nt-cli commands") && !strings.Contains(stdout, "save") {
		t.Fatalf("expected usage on stdout, got %q", stdout)
	}
}

// --- assertion-shape helper used by parity tests ---------------------------

// mustJSON unmarshals a JSON object string for parity comparisons; fails the
// test if the input is not a JSON object.
func mustJSON(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("expected JSON object, got %q (err %v)", raw, err)
	}
	return m
}
