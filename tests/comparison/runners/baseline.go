package runners

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/code-index-for-llms/code-index/pkg/types"
)

// BaselineRunner models Claude with no indexing plugin: every query returns
// the entire fixture codebase as context.  ContextTokens() reflects the full
// concatenated size of all source files - the worst-case token baseline.
type BaselineRunner struct {
	fixtureDir string
	allContent string // concatenated content of every file
	allResults []types.SearchResult
	counter    TokenCounter
}

// NewBaselineRunner creates a BaselineRunner with the given token counter.
func NewBaselineRunner(counter TokenCounter) *BaselineRunner {
	return &BaselineRunner{counter: counter}
}

func (r *BaselineRunner) Name() string { return "baseline" }

// Setup reads every non-hidden file in fixtureDir and stores the content.
func (r *BaselineRunner) Setup(fixtureDir string) error {
	r.fixtureDir = fixtureDir
	var sb strings.Builder
	var results []types.SearchResult

	err := filepath.WalkDir(fixtureDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		rel, _ := filepath.Rel(fixtureDir, path)
		rel = filepath.ToSlash(rel)

		header := fmt.Sprintf("\n// %s\n", rel)
		sb.WriteString(header)
		sb.Write(content)

		results = append(results, types.SearchResult{
			Chunk: types.Chunk{
				FilePath:  rel,
				Name:      rel,
				Content:   string(content),
				ChunkType: types.ChunkTypeFile,
			},
			Score: 1.0,
		})
		return nil
	})
	if err != nil {
		return fmt.Errorf("baseline setup walk: %w", err)
	}

	r.allContent = sb.String()
	r.allResults = results
	return nil
}

// Search returns all fixture files for every query - the baseline always
// provides "everything" as context.
func (r *BaselineRunner) Search(_ string, _ int) ([]types.SearchResult, time.Duration, error) {
	start := time.Now()
	// Simulate a trivial "search" (just return all pre-loaded results).
	results := make([]types.SearchResult, len(r.allResults))
	copy(results, r.allResults)
	return results, time.Since(start), nil
}

// ContextTokens returns the token count of the entire fixture codebase.
// The topK parameter is ignored: the baseline always sends everything.
func (r *BaselineRunner) ContextTokens(_ []types.SearchResult) int {
	return r.counter.Count(r.allContent)
}

// OutputTokensEstimate returns the baseline estimate unchanged (no compression).
func (r *BaselineRunner) OutputTokensEstimate(baselineOutputTokens int) int {
	return baselineOutputTokens
}

func (r *BaselineRunner) Cleanup() error { return nil }
