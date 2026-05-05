// Package comparison provides the test harness and report generation for the
// code-index-for-llms comparison benchmark.
package comparison

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/code-index-for-llms/code-index/pkg/types"
	"github.com/code-index-for-llms/code-index/tests/comparison/runners"
)

// ─── Query set types ──────────────────────────────────────────────────────────

// RelevantChunk is a single annotated ground-truth chunk for a query.
type RelevantChunk struct {
	FilePath  string `json:"file_path"`
	ChunkName string `json:"chunk_name"`
	Grade     int    `json:"grade"`
}

// Query is one entry from queries.json.
type Query struct {
	ID             string          `json:"id"`
	Text           string          `json:"text"`
	Category       string          `json:"category"`
	LanguageHint   *string         `json:"language_hint"`
	RelevantChunks []RelevantChunk `json:"relevant_chunks"`
	Notes          string          `json:"notes"`
}

// RelevanceSet is the ground-truth for one query.
// Key format: "file_path::chunk_name" (lower-case file paths, relative to fixture root).
// Value: relevance grade (1 = relevant, 2 = highly relevant).
type RelevanceSet map[string]int

// QuerySet is the root of queries.json.
type QuerySet struct {
	Version     string  `json:"version"`
	Description string  `json:"description"`
	Queries     []Query `json:"queries"`
}

// LoadQuerySet reads and parses queries.json from the given path.
func LoadQuerySet(path string) (QuerySet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return QuerySet{}, fmt.Errorf("read query set: %w", err)
	}
	var qs QuerySet
	if err := json.Unmarshal(data, &qs); err != nil {
		return QuerySet{}, fmt.Errorf("parse query set: %w", err)
	}
	return qs, nil
}

// ─── Per-runner results ───────────────────────────────────────────────────────

// QueryMetrics holds all computed metrics for a single (runner, query) pair.
type QueryMetrics struct {
	QueryID         string
	QueryText       string
	LanguageHint    *string // propagated from Query for filtering
	RecallAt3       float64
	RecallAt5       float64
	RecallAt10      float64
	MRR             float64
	NDCGAt10        float64
	ContextTokens   int
	OutputTokens    int
	SearchLatencyMs int64
	TopResults      []ResultSummary
	Error           string
}

// ResultSummary is a compact representation of a search result for reports.
type ResultSummary struct {
	FilePath  string  `json:"file_path"`
	ChunkName string  `json:"chunk_name"`
	Score     float64 `json:"score"`
}

// RunnerReport aggregates all queries for one runner.
type RunnerReport struct {
	Name            string
	Status          string // "ok", "skipped", "error"
	SetupDurationMs int64
	IndexSizeBytes  int64
	Queries         []QueryMetrics
	// Aggregates (filled after all queries complete).
	// When ExcludeLanguages is set, aggregates cover only non-excluded queries.
	// TotalQueryCount is always the full count; AggQueryCount is the filtered count.
	ExcludeLanguages   []string // language hints to skip when computing aggregates
	FilterNote         string   // human-readable note shown in reports, e.g. "excl. terraform"
	TotalQueryCount    int
	AggQueryCount      int // queries included in aggregate (≤ TotalQueryCount)
	AvgRecallAt3       float64
	AvgRecallAt5       float64
	AvgRecallAt10      float64
	AvgMRR             float64
	AvgNDCGAt10        float64
	AvgContextTokens   float64
	AvgOutputTokens    float64
	AvgSearchLatencyMs float64
}

// ─── Harness ──────────────────────────────────────────────────────────────────

// Config controls harness behaviour.
type Config struct {
	FixtureDir              string
	QuerySetPath            string
	TopK                    int
	BaselineOutputTokensEst int // typical LLM response size (tokens) without compression
	DBDir                   string // directory for temporary runner databases
	// ExcludeLanguages maps runner name → language hints to exclude from aggregate metrics.
	// All queries are still executed and appear in per-query results; only the aggregate is filtered.
	ExcludeLanguages map[string][]string
}

// Run executes all runners against all queries and returns per-runner reports.
// Runners are set up in parallel; queries within each runner are sequential
// to avoid SQLite write contention.
func Run(cfg Config, runnerList []runners.Runner) ([]RunnerReport, error) {
	qs, err := LoadQuerySet(cfg.QuerySetPath)
	if err != nil {
		return nil, err
	}
	if cfg.TopK <= 0 {
		cfg.TopK = 10
	}
	if cfg.BaselineOutputTokensEst <= 0 {
		cfg.BaselineOutputTokensEst = 800
	}

	var mu sync.Mutex
	reports := make([]RunnerReport, len(runnerList))
	var wg sync.WaitGroup

	for idx, runner := range runnerList {
		wg.Add(1)
		go func(i int, r runners.Runner) {
			defer wg.Done()
			report := runOneRunner(r, cfg, qs)
			mu.Lock()
			reports[i] = report
			mu.Unlock()
		}(idx, runner)
	}
	wg.Wait()

	return reports, nil
}

func runOneRunner(r runners.Runner, cfg Config, qs QuerySet) RunnerReport {
	report := RunnerReport{Name: r.Name()}

	setupStart := time.Now()
	err := r.Setup(cfg.FixtureDir)
	report.SetupDurationMs = time.Since(setupStart).Milliseconds()

	if err != nil {
		if errors.Is(err, runners.ErrRunnerUnavailable) {
			report.Status = "skipped"
			return report
		}
		report.Status = "error"
		report.Queries = []QueryMetrics{{Error: err.Error()}}
		return report
	}
	report.Status = "ok"

	if dbPath := cfg.DBDir + "/" + r.Name() + ".db"; cfg.DBDir != "" {
		if info, err := os.Stat(dbPath); err == nil {
			report.IndexSizeBytes = info.Size()
		}
	}

	// Configure per-runner language exclusions.
	if langs := cfg.ExcludeLanguages[r.Name()]; len(langs) > 0 {
		report.ExcludeLanguages = langs
		report.FilterNote = "excl. " + strings.Join(langs, ", ")
	}

	defer r.Cleanup() //nolint:errcheck

	for _, q := range qs.Queries {
		qm := runOneQuery(r, q, cfg)
		report.Queries = append(report.Queries, qm)
	}
	report.TotalQueryCount = len(report.Queries)

	aggregateReport(&report)
	return report
}

func runOneQuery(r runners.Runner, q Query, cfg Config) QueryMetrics {
	qm := QueryMetrics{
		QueryID:      q.ID,
		QueryText:    q.Text,
		LanguageHint: q.LanguageHint,
	}

	results, latency, err := r.Search(q.Text, cfg.TopK)
	qm.SearchLatencyMs = latency.Milliseconds()
	if err != nil {
		qm.Error = err.Error()
		return qm
	}

	// Build normalised relevance set (lower-case file paths for case-insensitive match).
	rs := BuildRelevanceSet(q.RelevantChunks)

	// Result-based metrics: FILE-type chunks (baseline) match any relevant chunk
	// in the same file, so baseline correctly scores ~1.0 recall.
	qm.RecallAt3 = recallAtKResults(results, rs, 3)
	qm.RecallAt5 = recallAtKResults(results, rs, 5)
	qm.RecallAt10 = recallAtKResults(results, rs, cfg.TopK)
	qm.MRR = mrrResults(results, rs)
	qm.NDCGAt10 = ndcgAtKResults(results, rs, cfg.TopK)
	qm.ContextTokens = r.ContextTokens(results)
	qm.OutputTokens = r.OutputTokensEstimate(cfg.BaselineOutputTokensEst)

	top := results
	if len(top) > 5 {
		top = top[:5]
	}
	for _, res := range top {
		qm.TopResults = append(qm.TopResults, ResultSummary{
			FilePath:  res.Chunk.FilePath,
			ChunkName: res.Chunk.Name,
			Score:     res.Score,
		})
	}

	return qm
}

// ─── metric helpers ───────────────────────────────────────────────────────────

// fileRelevanceSet maps lower-cased file paths to the max relevance grade of
// any relevant chunk in that file. Used for file-level result matching.
func fileRelevanceSet(rs RelevanceSet) map[string]int {
	fm := make(map[string]int, len(rs))
	for key, grade := range rs {
		if i := strings.Index(key, "::"); i >= 0 {
			fp := key[:i]
			if grade > fm[fp] {
				fm[fp] = grade
			}
		}
	}
	return fm
}

// resultGrade returns the relevance grade for a single search result.
// FILE-type chunks match any relevant chunk in the same file (using max grade).
func resultGrade(res types.SearchResult, rs RelevanceSet, frs map[string]int) float64 {
	if res.Chunk.ChunkType == types.ChunkTypeFile {
		fp := strings.ToLower(res.Chunk.FilePath)
		return float64(frs[fp])
	}
	return float64(rs[NormalizeKey(res)])
}

func recallAtKResults(results []types.SearchResult, rs RelevanceSet, k int) float64 {
	if len(rs) == 0 {
		return 1.0
	}
	found := map[string]bool{}
	for i, res := range results {
		isFile := res.Chunk.ChunkType == types.ChunkTypeFile
		// FILE-type results (baseline) carry the entire file regardless of rank:
		// the LLM receives all of them so the k cutoff does not apply.
		if !isFile && i >= k {
			break
		}
		if isFile {
			fp := strings.ToLower(res.Chunk.FilePath)
			for rk := range rs {
				if strings.HasPrefix(rk, fp+"::") {
					found[rk] = true
				}
			}
		} else {
			key := NormalizeKey(res)
			if _, ok := rs[key]; ok {
				found[key] = true
			}
		}
	}
	return float64(len(found)) / float64(len(rs))
}

func mrrResults(results []types.SearchResult, rs RelevanceSet) float64 {
	frs := fileRelevanceSet(rs)
	// For FILE-type runners (baseline): all files are in context, so if any
	// relevant file exists in results, MRR = 1.0 (rank is irrelevant).
	allFile := len(results) > 0 && results[0].Chunk.ChunkType == types.ChunkTypeFile
	if allFile {
		for _, res := range results {
			fp := strings.ToLower(res.Chunk.FilePath)
			if frs[fp] > 0 {
				return 1.0
			}
		}
		return 0.0
	}
	for i, res := range results {
		if resultGrade(res, rs, frs) > 0 {
			return 1.0 / float64(i+1)
		}
	}
	return 0.0
}

func ndcgAtKResults(results []types.SearchResult, rs RelevanceSet, k int) float64 {
	if k > len(results) {
		k = len(results)
	}
	frs := fileRelevanceSet(rs)
	dcg := 0.0
	for i := 0; i < k; i++ {
		grade := resultGrade(results[i], rs, frs)
		if grade > 0 {
			dcg += grade / math.Log2(float64(i+2))
		}
	}
	// Ideal DCG: sorted grades descending.
	grades := make([]float64, 0, len(rs))
	for _, g := range rs {
		grades = append(grades, float64(g))
	}
	for i := 1; i < len(grades); i++ {
		for j := i; j > 0 && grades[j] > grades[j-1]; j-- {
			grades[j], grades[j-1] = grades[j-1], grades[j]
		}
	}
	idcg := 0.0
	for i := 0; i < k && i < len(grades); i++ {
		idcg += grades[i] / math.Log2(float64(i+2))
	}
	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

// key-based variants kept for TestMetrics unit tests.
func recallAtKKeys(keys []string, relevant RelevanceSet, k int) float64 {
	if len(relevant) == 0 {
		return 1.0
	}
	if k > len(keys) {
		k = len(keys)
	}
	found := 0
	for i := 0; i < k; i++ {
		if _, ok := relevant[keys[i]]; ok {
			found++
		}
	}
	return float64(found) / float64(len(relevant))
}

func mrrKeys(keys []string, relevant RelevanceSet) float64 {
	for i, k := range keys {
		if _, ok := relevant[k]; ok {
			return 1.0 / float64(i+1)
		}
	}
	return 0.0
}

func ndcgAtKKeys(keys []string, relevant RelevanceSet, k int) float64 {
	if k > len(keys) {
		k = len(keys)
	}
	dcg := 0.0
	for i := 0; i < k; i++ {
		grade := float64(relevant[keys[i]])
		if grade > 0 {
			dcg += grade / math.Log2(float64(i+2))
		}
	}
	grades := make([]float64, 0, len(relevant))
	for _, g := range relevant {
		grades = append(grades, float64(g))
	}
	for i := 1; i < len(grades); i++ {
		for j := i; j > 0 && grades[j] > grades[j-1]; j-- {
			grades[j], grades[j-1] = grades[j-1], grades[j]
		}
	}
	idcg := 0.0
	for i := 0; i < k && i < len(grades); i++ {
		idcg += grades[i] / math.Log2(float64(i+2))
	}
	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

// ─── aggregation ─────────────────────────────────────────────────────────────

func aggregateReport(r *RunnerReport) {
	excludeSet := make(map[string]bool, len(r.ExcludeLanguages))
	for _, l := range r.ExcludeLanguages {
		excludeSet[strings.ToLower(l)] = true
	}

	n := 0
	for _, q := range r.Queries {
		if q.Error != "" {
			continue
		}
		if q.LanguageHint != nil && excludeSet[strings.ToLower(*q.LanguageHint)] {
			continue
		}
		n++
		r.AvgRecallAt3 += q.RecallAt3
		r.AvgRecallAt5 += q.RecallAt5
		r.AvgRecallAt10 += q.RecallAt10
		r.AvgMRR += q.MRR
		r.AvgNDCGAt10 += q.NDCGAt10
		r.AvgContextTokens += float64(q.ContextTokens)
		r.AvgOutputTokens += float64(q.OutputTokens)
		r.AvgSearchLatencyMs += float64(q.SearchLatencyMs)
	}
	r.AggQueryCount = n
	if n == 0 {
		return
	}
	fn := float64(n)
	r.AvgRecallAt3 /= fn
	r.AvgRecallAt5 /= fn
	r.AvgRecallAt10 /= fn
	r.AvgMRR /= fn
	r.AvgNDCGAt10 /= fn
	r.AvgContextTokens /= fn
	r.AvgOutputTokens /= fn
	r.AvgSearchLatencyMs /= fn
}

// NormalizeKey lower-cases the file path component for case-insensitive matching.
func NormalizeKey(r types.SearchResult) string {
	fp := r.Chunk.FilePath
	// lower-case for case-insensitive file path comparison
	result := ""
	for _, c := range fp {
		if c >= 'A' && c <= 'Z' {
			result += string(rune(c + 32))
		} else {
			result += string(c)
		}
	}
	return result + "::" + r.Chunk.Name
}
