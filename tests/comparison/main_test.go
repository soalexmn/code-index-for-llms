// Package comparison runs the code-index-for-llms comparison benchmark.
//
// Usage:
//
//	go test ./tests/comparison/... -v -timeout 10m
//	go test ./tests/comparison/... -v -timeout 10m -run TestComparison
//	go test ./tests/comparison/... -v -timeout 15m -run TestComparisonGin
//
// Output is written to tests/comparison/output/ by default.
// Set OUTPUT_DIR env var to override.
//
// TestComparisonGin clones gin-gonic/gin into tests/fixtures/gin/ on first run.
// Set FIXTURE_GIN_DIR to point at an existing checkout instead.
package comparison

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/code-index-for-llms/code-index/tests/comparison/runners"
)

// TestComparison is the main entry point for the comparison benchmark.
func TestComparison(t *testing.T) {
	// Locate paths relative to this test file.
	_, thisFile, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(thisFile)
	fixtureDir := filepath.Join(testDir, "..", "fixtures", "sample_project")
	querySetPath := filepath.Join(testDir, "testdata", "queries.json")
	outputDir := filepath.Join(testDir, "output")
	if env := os.Getenv("OUTPUT_DIR"); env != "" {
		outputDir = env
	}

	// Temp dir for runner SQLite databases.
	dbDir := t.TempDir()

	counter := runners.DefaultTokenCounter

	runnerList := []runners.Runner{
		runners.NewBaselineRunner(counter),
		runners.NewCavemanRunner(counter, 0.25),
		runners.NewCCERunner(counter),
		runners.NewCILRunner(filepath.Join(dbDir, "cil.db"), counter),
	}

	cfg := Config{
		FixtureDir:              fixtureDir,
		QuerySetPath:            querySetPath,
		TopK:                    10,
		BaselineOutputTokensEst: 800,
		DBDir:                   dbDir,
		// CCE cannot parse Terraform; exclude those queries from its aggregate
		// so the comparison reflects only languages CCE actually supports.
		// CIL always aggregates over all queries.
		ExcludeLanguages: map[string][]string{
			"cce": {"terraform"},
		},
	}

	t.Logf("fixture:    %s", fixtureDir)
	t.Logf("query set:  %s", querySetPath)
	t.Logf("output dir: %s", outputDir)

	reports, err := Run(cfg, runnerList)
	if err != nil {
		t.Fatalf("harness: %v", err)
	}

	// Print a quick summary table.
	t.Log("\n=== COMPARISON RESULTS ===")
	t.Logf("%-12s %-8s %10s %10s %10s %10s %16s %12s",
		"Runner", "Status", "Recall@3", "Recall@5", "Recall@10", "MRR",
		"Context Tokens", "Latency(ms)")
	t.Logf("%s", "────────────────────────────────────────────────────────────────────────────────────")
	for _, r := range reports {
		if r.Status != "ok" {
			t.Logf("%-12s %-8s  (skipped or error)", r.Name, r.Status)
			continue
		}
		t.Logf("%-12s %-8s %10.2f %10.2f %10.2f %10.2f %16.0f %12.1f",
			r.Name, r.Status,
			r.AvgRecallAt3, r.AvgRecallAt5, r.AvgRecallAt10, r.AvgMRR,
			r.AvgContextTokens, r.AvgSearchLatencyMs)
	}

	// Build and write reports.
	qs, _ := LoadQuerySet(querySetPath)
	jr := BuildJSONReport(reports, fixtureDir, len(qs.Queries))

	if err := WriteJSONReport(jr, outputDir); err != nil {
		t.Errorf("write JSON report: %v", err)
	} else {
		t.Logf("JSON report: %s/results.json", outputDir)
	}

	if err := WriteMarkdownReport(reports, jr, outputDir); err != nil {
		t.Errorf("write Markdown report: %v", err)
	} else {
		t.Logf("Markdown report: %s/report.md", outputDir)
	}

	// Fail the test only if our own runner (CIL) produced errors on every query.
	cilErrors := 0
	for _, r := range reports {
		if r.Name == "cil" {
			if r.Status == "error" {
				t.Errorf("CIL runner setup failed")
				break
			}
			for _, q := range r.Queries {
				if q.Error != "" {
					cilErrors++
					t.Errorf("CIL query %s failed: %s", q.QueryID, q.Error)
				}
			}
		}
	}

	// Sanity check: CIL index should not be completely empty.
	// Low recall for keyword-style queries is expected with BM25-only search;
	// zero results on ALL queries indicates an index build failure.
	for _, r := range reports {
		if r.Name != "cil" || r.Status != "ok" {
			continue
		}
		noResults := 0
		for _, q := range r.Queries {
			if q.Error == "" && len(q.TopResults) == 0 {
				noResults++
			}
		}
		t.Logf("CIL: %d/%d queries returned no results (BM25 requires exact token match)", noResults, len(r.Queries))
		if noResults == len(r.Queries) {
			t.Errorf("CIL returned NO results for ANY query - index is likely empty")
		}
	}
}

// TestComparisonGin runs the benchmark against gin-gonic/gin (a real-world Go project).
// On first run it clones the repo into tests/fixtures/gin/; subsequent runs reuse the clone.
// Set FIXTURE_GIN_DIR to point at an existing gin checkout to skip the clone.
func TestComparisonGin(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(thisFile)

	fixtureDir := filepath.Join(testDir, "..", "fixtures", "gin")
	if env := os.Getenv("FIXTURE_GIN_DIR"); env != "" {
		fixtureDir = env
	}

	// Clone if the fixture is not already present.
	if _, err := os.Stat(fixtureDir); os.IsNotExist(err) {
		t.Log("Cloning gin-gonic/gin v1.10.0 (first run only)...")
		cmd := exec.Command("git", "clone", "--depth=1", "--branch=v1.10.0",
			"https://github.com/gin-gonic/gin.git", fixtureDir)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			t.Skipf("could not clone gin fixture (set FIXTURE_GIN_DIR to use existing checkout): %v", err)
		}
	}

	querySetPath := filepath.Join(testDir, "testdata", "queries_gin.json")
	outputDir := filepath.Join(testDir, "output", "gin")
	if env := os.Getenv("OUTPUT_DIR"); env != "" {
		outputDir = filepath.Join(env, "gin")
	}

	dbDir := t.TempDir()
	counter := runners.DefaultTokenCounter

	runnerList := []runners.Runner{
		runners.NewBaselineRunner(counter),
		runners.NewCCERunner(counter),
		runners.NewCILRunner(filepath.Join(dbDir, "cil.db"), counter),
	}

	cfg := Config{
		FixtureDir:              fixtureDir,
		QuerySetPath:            querySetPath,
		TopK:                    10,
		BaselineOutputTokensEst: 800,
		DBDir:                   dbDir,
	}

	t.Logf("fixture:    %s", fixtureDir)
	t.Logf("query set:  %s", querySetPath)
	t.Logf("output dir: %s", outputDir)

	reports, err := Run(cfg, runnerList)
	if err != nil {
		t.Fatalf("harness: %v", err)
	}

	t.Log("\n=== GIN COMPARISON RESULTS ===")
	t.Logf("%-12s %-8s %10s %10s %10s %10s %16s %12s",
		"Runner", "Status", "Recall@3", "Recall@5", "Recall@10", "MRR",
		"Context Tokens", "Latency(ms)")
	t.Logf("%s", "────────────────────────────────────────────────────────────────────────────────────")
	for _, r := range reports {
		if r.Status != "ok" {
			t.Logf("%-12s %-8s  (skipped or error)", r.Name, r.Status)
			continue
		}
		t.Logf("%-12s %-8s %10.2f %10.2f %10.2f %10.2f %16.0f %12.1f",
			r.Name, r.Status,
			r.AvgRecallAt3, r.AvgRecallAt5, r.AvgRecallAt10, r.AvgMRR,
			r.AvgContextTokens, r.AvgSearchLatencyMs)
	}

	qs, _ := LoadQuerySet(querySetPath)
	jr := BuildJSONReport(reports, fixtureDir, len(qs.Queries))

	if err := WriteJSONReport(jr, outputDir); err != nil {
		t.Errorf("write JSON report: %v", err)
	} else {
		t.Logf("JSON report: %s/results.json", outputDir)
	}
	if err := WriteMarkdownReport(reports, jr, outputDir); err != nil {
		t.Errorf("write Markdown report: %v", err)
	} else {
		t.Logf("Markdown report: %s/report.md", outputDir)
	}

	for _, r := range reports {
		if r.Name != "cil" || r.Status != "ok" {
			continue
		}
		noResults := 0
		for _, q := range r.Queries {
			if q.Error == "" && len(q.TopResults) == 0 {
				noResults++
			}
		}
		t.Logf("CIL: %d/%d queries returned no results", noResults, len(r.Queries))
		if noResults == len(r.Queries) {
			t.Errorf("CIL returned NO results for ANY query - index is likely empty")
		}
	}
}

// TestMetrics verifies the metric calculation functions with known inputs.
func TestMetrics(t *testing.T) {
	t.Run("RecallAtK", func(t *testing.T) {
		rs := RelevanceSet{"a::foo": 2, "b::bar": 1}
		// Perfect top-2: both relevant.
		keys := []string{"a::foo", "b::bar", "c::baz"}
		got := recallAtKKeys(keys, rs, 2)
		if got != 1.0 {
			t.Errorf("Recall@2 want 1.0 got %.2f", got)
		}
		// Only 1 of 2 in top-1.
		got = recallAtKKeys(keys, rs, 1)
		if got != 0.5 {
			t.Errorf("Recall@1 want 0.5 got %.2f", got)
		}
	})

	t.Run("MRR", func(t *testing.T) {
		rs := RelevanceSet{"b::bar": 1}
		keys := []string{"a::foo", "b::bar", "c::baz"}
		got := mrrKeys(keys, rs)
		want := 1.0 / 2.0
		if fmt.Sprintf("%.3f", got) != fmt.Sprintf("%.3f", want) {
			t.Errorf("MRR want %.3f got %.3f", want, got)
		}
	})

	t.Run("NDCG", func(t *testing.T) {
		// All relevant items at top: nDCG should be 1.0.
		rs := RelevanceSet{"a::foo": 2}
		keys := []string{"a::foo", "b::bar"}
		got := ndcgAtKKeys(keys, rs, 10)
		if got != 1.0 {
			t.Errorf("nDCG@10 want 1.0 got %.3f", got)
		}
		// Relevant item at rank 2: nDCG < 1.0.
		keys2 := []string{"b::bar", "a::foo"}
		got2 := ndcgAtKKeys(keys2, rs, 10)
		if got2 >= 1.0 || got2 <= 0 {
			t.Errorf("nDCG@10 (rank 2) want (0,1) got %.3f", got2)
		}
	})

	t.Run("TokenCounter", func(t *testing.T) {
		c := runners.SimpleTokenCounter{}
		got := c.Count("hello world") // 11 chars → 2 tokens
		if got < 1 {
			t.Errorf("token count want >=1 got %d", got)
		}
	})
}
