package store

import (
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// seedLargeStore inserts n synthetic rows with predictable token distribution.
// We insert in a single transaction to keep setup time bounded — without
// this, modernc.org/sqlite spends most of seed time on per-row fsyncs and
// the benchmark balloons to multi-minute runs on first execution.
func seedLargeStore(tb testing.TB, n int) *SQLiteStore {
	tb.Helper()
	dir := tb.TempDir()
	path := filepath.Join(dir, "bench.db")
	s, err := NewSQLiteStore(path)
	if err != nil {
		tb.Fatalf("open: %v", err)
	}
	if err := s.Init(); err != nil {
		tb.Fatalf("init: %v", err)
	}
	tb.Cleanup(func() { _ = s.Close() })

	// Keep token universe small so queries reliably hit a meaningful
	// fraction of rows — bm25 ranking only matters when there are
	// candidates to rank.
	universe := []string{
		"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta",
		"iota", "kappa", "lambda", "mu", "nu", "xi", "omicron", "pi",
	}
	rng := rand.New(rand.NewSource(42))
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tx, err := s.db.Begin()
	if err != nil {
		tb.Fatalf("begin: %v", err)
	}
	stmt, err := tx.Prepare(
		`INSERT INTO memory_items(content, created_at, updated_at) VALUES(?, ?, ?)`,
	)
	if err != nil {
		_ = tx.Rollback()
		tb.Fatalf("prepare: %v", err)
	}
	for i := 0; i < n; i++ {
		// 5–10 tokens per row, drawn from the universe.
		nt := 5 + rng.Intn(6)
		tokens := make([]byte, 0, 64)
		for j := 0; j < nt; j++ {
			if j > 0 {
				tokens = append(tokens, ' ')
			}
			tokens = append(tokens, universe[rng.Intn(len(universe))]...)
		}
		stamp := base.Add(time.Duration(i) * time.Second).Format(time.RFC3339)
		if _, err := stmt.Exec(string(tokens), stamp, stamp); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			tb.Fatalf("insert %d: %v", i, err)
		}
	}
	_ = stmt.Close()
	if err := tx.Commit(); err != nil {
		tb.Fatalf("commit: %v", err)
	}
	// Triggers don't fire on bulk-tx INSERTs through the prepared stmt
	// reliably across drivers; force a full FTS rebuild so the bench
	// measures actual ranked recall over the real corpus.
	if _, err := s.db.Exec(`INSERT INTO memory_fts(memory_fts) VALUES('rebuild')`); err != nil {
		tb.Fatalf("fts rebuild: %v", err)
	}
	return s
}

// TestRecall_P95Under50ms covers spec scenario "10k-row latency benchmark
// passes": with 10,000 rows seeded, p95 of Recall() must be < 50ms.
//
// This test is heavy (~2s on a laptop). It runs by default but can be
// skipped via NTCLI_SKIP_LATENCY=1 in constrained CI lanes. Failure here
// signals a real perf regression — do NOT silence it without investigation.
func TestRecall_P95Under50ms(t *testing.T) {
	if os.Getenv("NTCLI_SKIP_LATENCY") == "1" {
		t.Skip("NTCLI_SKIP_LATENCY=1 set")
	}
	const (
		rows  = 10_000
		runs  = 200
		limit = 10
	)
	s := seedLargeStore(t, rows)

	// Realistic query mix — single-token, multi-token, miss, common term.
	queries := []string{
		"alpha", "beta gamma", "delta epsilon zeta",
		"nonexistentterm", "lambda", "mu nu", "xi", "omicron pi",
		"alpha beta", "kappa",
	}
	samples := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		q := queries[i%len(queries)]
		start := time.Now()
		if _, err := s.Recall(q, limit); err != nil {
			t.Fatalf("recall %q: %v", q, err)
		}
		samples = append(samples, time.Since(start))
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	p95 := samples[int(float64(len(samples))*0.95)]
	median := samples[len(samples)/2]

	t.Logf("recall latency over %d rows × %d runs: median=%s p95=%s", rows, runs, median, p95)
	if p95 > 50*time.Millisecond {
		t.Fatalf("p95 latency %s exceeds 50ms SLO (median=%s, %d rows)", p95, median, rows)
	}
	if !s.UseFTS() {
		t.Fatalf("expected FTS active for latency benchmark; LIKE fallback is not the path under test")
	}
}

// BenchmarkRecall_FTS exposes a `go test -bench` entry point for finer-grained
// performance work. It deliberately seeds in the benchmark setup, not via b.N,
// so each iteration measures one real Recall against a stable 10k-row corpus.
func BenchmarkRecall_FTS(b *testing.B) {
	s := seedLargeStore(b, 10_000)
	queries := []string{"alpha", "beta gamma", "lambda mu", "xi omicron pi"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Recall(queries[i%len(queries)], 10); err != nil {
			b.Fatalf("recall: %v", err)
		}
	}
}
