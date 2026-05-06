package parity

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

// fakeClock returns timestamps from a pre-seeded sequence so replay
// latency is deterministic. Each Now() call advances the cursor.
type fakeClock struct {
	seq []time.Time
	i   int
}

func (c *fakeClock) Now() time.Time {
	t := c.seq[c.i]
	c.i++
	return t
}

// fakeRecaller pretends to be a store that returns a canned set of
// content strings for a given query. Lookup is exact-match on the
// query so each fixture row is independent.
type fakeRecaller struct {
	hits map[string][]string
}

func (r *fakeRecaller) Recall(query string, _ int) ([]string, error) {
	return r.hits[query], nil
}

// makeClock builds a clock whose Now() yields t0, t0+d, t0+2d, ...
// For each replayed query the harness calls Now() exactly twice
// (start, end), so the per-query latency is exactly `d`.
func makeClock(t0 time.Time, perCall time.Duration, count int) *fakeClock {
	seq := make([]time.Time, count)
	for i := 0; i < count; i++ {
		seq[i] = t0.Add(time.Duration(i) * perCall)
	}
	return &fakeClock{seq: seq}
}

// TestLoadQueries_FixtureSuiteHasMinimumRows verifies the on-disk
// fixture under testdata/parity/queries.json carries ≥10 rows per
// the spec ("≥10 queries with expected top hits"). Loading is the
// only behaviour the loader has — no merging, no defaults.
func TestLoadQueries_FixtureSuiteHasMinimumRows(t *testing.T) {
	queries, err := LoadQueries("../../testdata/parity/queries.json")
	if err != nil {
		t.Fatalf("LoadQueries: %v", err)
	}
	if len(queries) < 10 {
		t.Fatalf("fixture must have ≥10 queries, got %d", len(queries))
	}
	for i, q := range queries {
		if q.Query == "" {
			t.Errorf("queries[%d].query is empty", i)
		}
		if q.ExpectedMarker == "" {
			t.Errorf("queries[%d].expected_marker is empty", i)
		}
	}
}

// TestComputeContinuity_HitRateMatchesExpectedMarkers exercises the
// happy path: every fixture row's expected_marker appears in the
// top-3 returned by the (faked) recaller, so hit_rate = 1.0.
func TestComputeContinuity_HitRateMatchesExpectedMarkers(t *testing.T) {
	queries := []ContinuityQuery{
		{Query: "auth", ExpectedMarker: "JWT"},
		{Query: "fts", ExpectedMarker: "FTS5"},
	}
	recaller := &fakeRecaller{hits: map[string][]string{
		"auth": {"JWT auth middleware", "session note", "extra"},
		"fts":  {"FTS5 mirror table", "extra", "extra"},
	}}
	clk := makeClock(time.Unix(0, 0).UTC(), 5*time.Millisecond, 5)

	baseline, err := ComputeContinuity(queries, recaller, clk)
	if err != nil {
		t.Fatalf("ComputeContinuity: %v", err)
	}
	if baseline.Count != 2 {
		t.Errorf("Count: want 2, got %d", baseline.Count)
	}
	if baseline.TopKHitRate != 1.0 {
		t.Errorf("TopKHitRate: want 1.0, got %v", baseline.TopKHitRate)
	}
	if baseline.MedianResumeMs != 5 {
		t.Errorf("MedianResumeMs: want 5, got %v", baseline.MedianResumeMs)
	}
	// p95 of two equal values is the same value.
	if baseline.P95ResumeMs != 5 {
		t.Errorf("P95ResumeMs: want 5, got %v", baseline.P95ResumeMs)
	}
}

// TestComputeContinuity_PartialMissReducesHitRate forces a different
// code path: one expected_marker is missing from the top-3, so
// hit_rate must be the proper fraction (1/2 = 0.5). This is the
// triangulation case that breaks any "always 1.0" Fake It.
func TestComputeContinuity_PartialMissReducesHitRate(t *testing.T) {
	queries := []ContinuityQuery{
		{Query: "auth", ExpectedMarker: "JWT"},
		{Query: "fts", ExpectedMarker: "FTS5"},
	}
	recaller := &fakeRecaller{hits: map[string][]string{
		"auth": {"JWT auth middleware", "extra", "extra"},
		"fts":  {"unrelated", "noise", "stuff"}, // expected marker absent
	}}
	clk := makeClock(time.Unix(0, 0).UTC(), 10*time.Millisecond, 5)

	baseline, err := ComputeContinuity(queries, recaller, clk)
	if err != nil {
		t.Fatalf("ComputeContinuity: %v", err)
	}
	if baseline.TopKHitRate != 0.5 {
		t.Errorf("TopKHitRate: want 0.5, got %v", baseline.TopKHitRate)
	}
	// Per-query hit flag must be exposed for debugging the runbook.
	if !baseline.Queries[0].Hit {
		t.Error("queries[0] (auth) should be a hit")
	}
	if baseline.Queries[1].Hit {
		t.Error("queries[1] (fts) should be a miss")
	}
}

// TestComputeContinuity_DeterministicReplayIsIdempotent — the spec's
// hard invariant. Same input → byte-identical baseline.json.
func TestComputeContinuity_DeterministicReplayIsIdempotent(t *testing.T) {
	queries := []ContinuityQuery{
		{Query: "auth", ExpectedMarker: "JWT"},
		{Query: "fts", ExpectedMarker: "FTS5"},
	}
	hits := map[string][]string{
		"auth": {"JWT auth middleware"},
		"fts":  {"FTS5 mirror table"},
	}
	t0 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	r1 := &fakeRecaller{hits: hits}
	c1 := makeClock(t0, 7*time.Millisecond, 5)
	b1, err := ComputeContinuity(queries, r1, c1)
	if err != nil {
		t.Fatal(err)
	}
	r2 := &fakeRecaller{hits: hits}
	c2 := makeClock(t0, 7*time.Millisecond, 5)
	b2, err := ComputeContinuity(queries, r2, c2)
	if err != nil {
		t.Fatal(err)
	}

	js1, _ := json.Marshal(b1)
	js2, _ := json.Marshal(b2)
	if string(js1) != string(js2) {
		t.Errorf("replay not deterministic:\n  a=%s\n  b=%s", js1, js2)
	}
}

// TestComputeContinuity_P95Bounded asserts that p95 never falls below
// the maximum observed sample (the bounded-error invariant — p95 must
// be a real measurement from the latency series, not a stub).
func TestComputeContinuity_P95Bounded(t *testing.T) {
	queries := []ContinuityQuery{
		{Query: "q1", ExpectedMarker: "x"},
		{Query: "q2", ExpectedMarker: "x"},
		{Query: "q3", ExpectedMarker: "x"},
		{Query: "q4", ExpectedMarker: "x"},
		{Query: "q5", ExpectedMarker: "x"},
	}
	recaller := &fakeRecaller{hits: map[string][]string{
		"q1": {"x"}, "q2": {"x"}, "q3": {"x"}, "q4": {"x"}, "q5": {"x"},
	}}
	// Latencies: 1ms, 2ms, 3ms, 4ms, 50ms (one slow tail). Builder
	// uses fixed step, so sequence start/end pairs must vary; build
	// manually so latencies are 1,2,3,4,50.
	t0 := time.Unix(0, 0).UTC()
	seq := []time.Time{
		t0, t0.Add(1 * time.Millisecond),
		t0.Add(10 * time.Millisecond), t0.Add(12 * time.Millisecond),
		t0.Add(20 * time.Millisecond), t0.Add(23 * time.Millisecond),
		t0.Add(30 * time.Millisecond), t0.Add(34 * time.Millisecond),
		t0.Add(40 * time.Millisecond), t0.Add(90 * time.Millisecond),
		t0.Add(100 * time.Millisecond), // generatedAt tick
	}
	clk := &fakeClock{seq: seq}

	baseline, err := ComputeContinuity(queries, recaller, clk)
	if err != nil {
		t.Fatal(err)
	}
	// Median of {1,2,3,4,50} is 3.
	if baseline.MedianResumeMs != 3 {
		t.Errorf("MedianResumeMs: want 3, got %v", baseline.MedianResumeMs)
	}
	// p95 over 5 samples (nearest-rank): ceil(0.95*5)=5 → 50ms.
	if baseline.P95ResumeMs != 50 {
		t.Errorf("P95ResumeMs: want 50, got %v", baseline.P95ResumeMs)
	}
}

// TestComputeContinuity_VersionStamped makes the contract version
// part of the baseline so PR5 can detect drift across replays.
func TestComputeContinuity_VersionStamped(t *testing.T) {
	queries := []ContinuityQuery{{Query: "q", ExpectedMarker: "x"}}
	recaller := &fakeRecaller{hits: map[string][]string{"q": {"x"}}}
	clk := makeClock(time.Unix(0, 0).UTC(), time.Millisecond, 3)
	b, err := ComputeContinuity(queries, recaller, clk)
	if err != nil {
		t.Fatal(err)
	}
	if b.Version != ContinuityContractVersion {
		t.Errorf("Version: want %q, got %q", ContinuityContractVersion, b.Version)
	}
	if b.GeneratedAt.IsZero() {
		t.Error("GeneratedAt must be set")
	}
}

// TestScoreKnowledgeContinuity_HitRateAndLatencyShapeScore wires the
// harness into the parity scorecard's knowledge-continuity dimension
// (task 2.3). Score is bounded [0,100] and:
//   - 100% hits + p95 well under 50ms budget → 100
//   - 0% hits → 0 (no recall ever found the right answer)
//   - p95 over 50ms budget caps the score even with perfect hits.
func TestScoreKnowledgeContinuity_HitRateAndLatencyShapeScore(t *testing.T) {
	cases := []struct {
		name    string
		hitRate float64
		p95Ms   int64
		want    int
	}{
		{"perfect", 1.0, 10, 100},
		{"zero hits", 0.0, 10, 0},
		{"half hits, fast", 0.5, 5, 50},
		{"perfect hits but slow p95 caps score", 1.0, 100, 50},
		{"perfect hits at exactly budget keeps full score", 1.0, 50, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := ContinuityBaseline{TopKHitRate: tc.hitRate, P95ResumeMs: tc.p95Ms}
			got := ScoreKnowledgeContinuity(b)
			if got != tc.want {
				t.Errorf("ScoreKnowledgeContinuity(hit=%v p95=%v): want %d got %d",
					tc.hitRate, tc.p95Ms, tc.want, got)
			}
		})
	}
}

// TestScoreKnowledgeContinuity_FeedsScorecardDimension proves the
// integration claim of task 2.3: the harness output drives the
// knowledge-continuity dimension inside ComputeScorecard. We pass a
// perfect baseline through ScoreKnowledgeContinuity and feed it as
// the dimension's signal — the resulting verdict must report
// knowledge-continuity pass=true with score 100.
func TestScoreKnowledgeContinuity_FeedsScorecardDimension(t *testing.T) {
	perfect := ContinuityBaseline{TopKHitRate: 1.0, P95ResumeMs: 5}
	score := ScoreKnowledgeContinuity(perfect)
	signals := ScorecardSignals{
		CoreOps:                100,
		MetadataRetrieval:      100,
		SessionWorkflow:        100,
		ImportExportBackup:     100,
		ReliabilityOperability: 100,
		KnowledgeContinuity:    score,
		UXAPIContract:          100,
		SoakDays:               14,
	}
	v := ComputeScorecard(signals)
	if v.Verdict != VerdictPass {
		t.Fatalf("verdict: want pass, got %s (hold=%q)", v.Verdict, v.HoldReason)
	}
	for _, d := range v.Dimensions {
		if d.Name == "knowledge-continuity" {
			if d.Score != 100 || !d.Pass {
				t.Errorf("knowledge-continuity: want score=100 pass=true, got %+v", d)
			}
			return
		}
	}
	t.Fatal("knowledge-continuity dimension missing from verdict")
}

// guard against accidental float-rounding drift in the score helper.
func TestScoreKnowledgeContinuity_FloatHitRateRounds(t *testing.T) {
	// 1/3 hit rate over 3 queries is 0.333... → score 33 (truncated).
	got := ScoreKnowledgeContinuity(ContinuityBaseline{TopKHitRate: 1.0 / 3.0, P95ResumeMs: 5})
	if got != 33 {
		t.Errorf("want 33, got %d", got)
	}
	// Sanity: the math we depend on
	if math.Abs(1.0/3.0*100-33.333) > 0.01 {
		t.Fatal("float assumption broke")
	}
}
