// Package runners defines the Runner interface and supporting types for the
// comparison benchmark. Each Runner wraps one "approach" (baseline, caveman,
// CCE, or code-index-for-llms) and exposes a uniform search API.
package runners

import (
	"errors"
	"time"

	"github.com/code-index-for-llms/code-index/pkg/types"
)

// ErrRunnerUnavailable is returned by Setup when a runner's external
// dependency (e.g. the "cce" binary) is not available in the current
// environment. The harness records the runner as "skipped", not "failed".
var ErrRunnerUnavailable = errors.New("runner dependency not available")

// Runner is the common interface that every comparison approach must implement.
// Each Runner manages its own state (index path, subprocess handle, etc.).
type Runner interface {
	// Name returns a stable snake_case identifier used in report keys.
	// Examples: "baseline", "caveman", "cce", "cil"
	Name() string

	// Setup initialises the runner against a fixture directory.
	// It may build an index, verify an external binary exists, or do nothing.
	// Returns ErrRunnerUnavailable if a required external tool is missing.
	// Setup must be idempotent: calling it twice must not corrupt state.
	Setup(fixtureDir string) error

	// Search returns the top-topK results for the given natural-language query
	// together with the wall-clock time spent in the search call itself.
	// Implementations must be safe for sequential calls on the same runner.
	Search(query string, topK int) ([]types.SearchResult, time.Duration, error)

	// ContextTokens returns the estimated LLM input tokens for the given
	// result set (i.e. the context that would be prepended to the prompt).
	ContextTokens(results []types.SearchResult) int

	// OutputTokensEstimate returns the estimated LLM output tokens.
	// For most runners this delegates to baselineOutputTokens unchanged.
	// CavemanRunner multiplies it by its documented compression factor.
	OutputTokensEstimate(baselineOutputTokens int) int

	// Cleanup releases all resources (closes DB, kills subprocesses, etc.).
	Cleanup() error
}

// RunnerResult is the per-(runner, query) data collected by the harness.
type RunnerResult struct {
	RunnerName      string
	QueryID         string
	Results         []types.SearchResult
	ContextTokens   int
	OutputTokens    int   // estimated
	SearchLatencyMs int64
	Error           string // non-empty when Search() returned an error
}
