package runners

import (
	"time"

	"github.com/code-index-for-llms/code-index/pkg/types"
)

// CavemanRunner models Claude + the caveman plugin.
// Caveman is an output-compression mode: it does not change what code context
// is retrieved - it compresses Claude's response tokens by ~75%.
// Therefore, ContextTokens() is identical to the baseline; only
// OutputTokensEstimate() applies the documented compression factor.
type CavemanRunner struct {
	base             *BaselineRunner
	compressionRatio float64 // fraction of output tokens retained (default 0.25)
}

// NewCavemanRunner creates a CavemanRunner wrapping a BaselineRunner.
// compressionRatio is the fraction of output tokens remaining after caveman
// compression (documented as ~0.25, i.e. 75% reduction).
func NewCavemanRunner(counter TokenCounter, compressionRatio float64) *CavemanRunner {
	if compressionRatio <= 0 || compressionRatio > 1 {
		compressionRatio = 0.25
	}
	return &CavemanRunner{
		base:             NewBaselineRunner(counter),
		compressionRatio: compressionRatio,
	}
}

func (r *CavemanRunner) Name() string { return "caveman" }

func (r *CavemanRunner) Setup(fixtureDir string) error {
	return r.base.Setup(fixtureDir)
}

// Search returns the same full-codebase results as the baseline.
// Caveman does not change retrieval - only Claude's verbosity.
func (r *CavemanRunner) Search(query string, topK int) ([]types.SearchResult, time.Duration, error) {
	return r.base.Search(query, topK)
}

// ContextTokens is identical to the baseline (caveman is output-only compression).
func (r *CavemanRunner) ContextTokens(results []types.SearchResult) int {
	return r.base.ContextTokens(results)
}

// OutputTokensEstimate applies the caveman compression ratio to the baseline
// output estimate to model the reduced response size.
func (r *CavemanRunner) OutputTokensEstimate(baselineOutputTokens int) int {
	return int(float64(baselineOutputTokens) * r.compressionRatio)
}

func (r *CavemanRunner) Cleanup() error { return r.base.Cleanup() }
