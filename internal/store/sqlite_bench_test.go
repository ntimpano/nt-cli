package store

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nt-cli/internal/app"
)

// perfTrials is the documented best-of-N count for the latency
// benches. Three trials are enough to absorb a single transient CPU
// spike from a parallel test without masking a real regression — if
// all three breach 50ms, the SLO is genuinely violated. Tunable via
// NTCLI_PERF_TRIALS for operators investigating regressions.
const perfTrials = 3

// resolvePerfTrials reads NTCLI_PERF_TRIALS as a positive integer, or
// returns perfTrials when unset/invalid. Pure-helper-style so future
// callers (graph bench, future benches) share one knob.
func resolvePerfTrials() int {
	raw := os.Getenv("NTCLI_PERF_TRIALS")
	if raw == "" {
		return perfTrials
	}
	// Use a small in-place parse to avoid pulling strconv at the top
	// just for this single read.
	n := 0
	for _, c := range raw {
		if c < '0' || c > '9' {
			return perfTrials
		}
		n = n*10 + int(c-'0')
		if n > 10 { // cap so a typo can't run 1e6 trials
			return perfTrials
		}
	}
	if n <= 0 {
		return perfTrials
	}
	return n
}

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
// Hardening (PR7): the SLO is unchanged at 50ms, but we now run the
// trial up to N times (default 3, override with NTCLI_PERF_TRIALS)
// and assert against the BEST p95. A single transient CPU spike from
// a parallel test no longer flakes the assertion; a real regression
// still does (every trial breaches). NTCLI_SKIP_LATENCY=1 still skips
// the test entirely for constrained CI lanes.
func TestRecall_P95Under50ms(t *testing.T) {
	if os.Getenv("NTCLI_SKIP_LATENCY") == "1" {
		t.Skip("NTCLI_SKIP_LATENCY=1 set")
	}
	const (
		rows  = 10_000
		runs  = 200
		limit = 10
	)
	// Realistic query mix — single-token, multi-token, miss, common term.
	queries := []string{
		"alpha", "beta gamma", "delta epsilon zeta",
		"nonexistentterm", "lambda", "mu nu", "xi", "omicron pi",
		"alpha beta", "kappa",
	}
	runOne := func() []time.Duration {
		s := seedLargeStore(t, rows)
		samples := make([]time.Duration, 0, runs)
		for i := 0; i < runs; i++ {
			q := queries[i%len(queries)]
			start := time.Now()
			if _, err := s.Recall(q, limit); err != nil {
				t.Fatalf("recall %q: %v", q, err)
			}
			samples = append(samples, time.Since(start))
		}
		if !s.UseFTS() {
			t.Fatalf("expected FTS active for latency benchmark; LIKE fallback is not the path under test")
		}
		return samples
	}
	trials := resolvePerfTrials()
	best := bestOfNTrials(trials, runOne)
	p95 := percentile(best, 0.95)
	median := percentile(best, 0.50)

	t.Logf("recall latency over %d rows × %d runs (best of %d trials): median=%s p95=%s",
		rows, runs, trials, median, p95)
	if p95 > 50*time.Millisecond {
		t.Fatalf("p95 latency %s exceeds 50ms SLO across %d trials (median=%s, %d rows)",
			p95, trials, median, rows)
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

// TestRecallGraphAware_P95Under50ms covers the PR4 scenario "graph-aware
// recall stays within latency SLO": with 10,000 rows and a sparse but
// realistic relation graph, p95 of RecallGraphAware MUST stay under
// the same 50ms budget as the legacy FTS path. The graph join + boost
// must not regress recall latency.
//
// PR7 hardening matches TestRecall_P95Under50ms: best-of-N trials so
// noise from parallel tests doesn't flake the assertion. The 50ms SLO
// is unchanged.
func TestRecallGraphAware_P95Under50ms(t *testing.T) {
	if os.Getenv("NTCLI_SKIP_LATENCY") == "1" {
		t.Skip("NTCLI_SKIP_LATENCY=1 set")
	}
	const (
		rows      = 10_000
		runs      = 200
		limit     = 10
		relations = 2_000 // ~20% of rows have at least one outbound edge
	)
	queries := []string{
		"alpha", "beta gamma", "delta epsilon zeta",
		"nonexistentterm", "lambda", "mu nu", "xi", "omicron pi",
		"alpha beta", "kappa",
	}
	runOne := func() []time.Duration {
		s := seedLargeStore(t, rows)
		// Seed a sparse relation graph spanning the corpus. We pick the
		// boostable subset (related, refines, depends_on) so the boost
		// arithmetic actually fires during recall.
		relTypes := []string{"related", "refines", "depends_on"}
		rng := rand.New(rand.NewSource(7))
		now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		for i := 0; i < relations; i++ {
			src := int64(1 + rng.Intn(rows))
			tgt := int64(1 + rng.Intn(rows))
			if src == tgt {
				continue
			}
			rt := relTypes[rng.Intn(len(relTypes))]
			if err := s.CreateRelation(src, tgt, rt, now.Add(time.Duration(i)*time.Second)); err != nil {
				// CreateRelation enforces uniqueness; collisions are
				// expected at this density and safe to skip.
				continue
			}
		}

		samples := make([]time.Duration, 0, runs)
		for i := 0; i < runs; i++ {
			opts := app.RecallOptions{Query: queries[i%len(queries)], Limit: limit, GraphAware: true}
			start := time.Now()
			if _, err := s.RecallGraphAware(opts); err != nil {
				t.Fatalf("recall graph-aware %q: %v", opts.Query, err)
			}
			samples = append(samples, time.Since(start))
		}
		if !s.UseFTS() {
			t.Fatalf("expected FTS active for graph-aware latency benchmark; LIKE fallback is not the path under test")
		}
		return samples
	}
	trials := resolvePerfTrials()
	best := bestOfNTrials(trials, runOne)
	p95 := percentile(best, 0.95)
	median := percentile(best, 0.50)

	t.Logf("graph-aware recall latency over %d rows + %d relations × %d runs (best of %d trials): median=%s p95=%s",
		rows, relations, runs, trials, median, p95)
	if p95 > 50*time.Millisecond {
		t.Fatalf("graph-aware p95 latency %s exceeds 50ms SLO across %d trials (median=%s, %d rows)",
			p95, trials, median, rows)
	}
}
