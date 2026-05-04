package comparison

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ─── JSON report ─────────────────────────────────────────────────────────────

// JSONReport is the machine-readable output artifact (results.json).
type JSONReport struct {
	RunAt        string             `json:"run_at"`
	FixtureDir   string             `json:"fixture_dir"`
	TotalQueries int                `json:"total_queries"`
	Runners      []JSONRunnerReport `json:"runners"`
	Comparison   ComparisonTable    `json:"comparison_table"`
}

// JSONRunnerReport is the per-runner section of the JSON report.
type JSONRunnerReport struct {
	Name            string            `json:"name"`
	Status          string            `json:"status"`
	SetupDurationMs int64             `json:"setup_duration_ms"`
	IndexSizeBytes  int64             `json:"index_size_bytes"`
	Aggregate       JSONAggregate     `json:"aggregate"`
	PerQuery        []JSONQueryResult `json:"per_query"`
}

// JSONAggregate holds the averaged metrics for a runner.
type JSONAggregate struct {
	RecallAt3          float64 `json:"recall_at_3"`
	RecallAt5          float64 `json:"recall_at_5"`
	RecallAt10         float64 `json:"recall_at_10"`
	MRR                float64 `json:"mrr"`
	NDCGAt10           float64 `json:"ndcg_at_10"`
	AvgContextTokens   float64 `json:"avg_context_tokens"`
	AvgOutputTokens    float64 `json:"avg_output_tokens_estimate"`
	AvgSearchLatencyMs float64 `json:"avg_search_latency_ms"`
}

// JSONQueryResult holds the per-query metrics.
type JSONQueryResult struct {
	QueryID              string          `json:"query_id"`
	QueryText            string          `json:"query_text"`
	RecallAt3            float64         `json:"recall_at_3"`
	RecallAt5            float64         `json:"recall_at_5"`
	RecallAt10           float64         `json:"recall_at_10"`
	MRR                  float64         `json:"mrr"`
	NDCGAt10             float64         `json:"ndcg_at_10"`
	ContextTokens        int             `json:"context_tokens"`
	OutputTokensEstimate int             `json:"output_tokens_estimate"`
	SearchLatencyMs      int64           `json:"search_latency_ms"`
	TopResults           []ResultSummary `json:"top_results"`
	Error                string          `json:"error,omitempty"`
}

// ComparisonTable holds cross-runner aggregated comparisons.
type ComparisonTable struct {
	TokenReductionVsBaseline map[string]float64 `json:"token_reduction_vs_baseline"`
	RecallAt10               map[string]float64 `json:"recall_at_10"`
	MRR                      map[string]float64 `json:"mrr"`
	AvgContextTokens         map[string]float64 `json:"avg_context_tokens"`
	AvgSearchLatencyMs       map[string]float64 `json:"avg_search_latency_ms"`
}

// BuildJSONReport converts RunnerReports into a JSONReport.
func BuildJSONReport(reports []RunnerReport, fixtureDir string, totalQueries int) JSONReport {
	jr := JSONReport{
		RunAt:        time.Now().UTC().Format(time.RFC3339),
		FixtureDir:   fixtureDir,
		TotalQueries: totalQueries,
		Comparison: ComparisonTable{
			TokenReductionVsBaseline: map[string]float64{},
			RecallAt10:               map[string]float64{},
			MRR:                      map[string]float64{},
			AvgContextTokens:         map[string]float64{},
			AvgSearchLatencyMs:       map[string]float64{},
		},
	}

	// Find baseline context tokens for reduction calculation.
	baselineTokens := 0.0
	for _, r := range reports {
		if r.Name == "baseline" && r.Status == "ok" {
			baselineTokens = r.AvgContextTokens
			break
		}
	}

	for _, r := range reports {
		jrr := JSONRunnerReport{
			Name:            r.Name,
			Status:          r.Status,
			SetupDurationMs: r.SetupDurationMs,
			IndexSizeBytes:  r.IndexSizeBytes,
			Aggregate: JSONAggregate{
				RecallAt3:          round2(r.AvgRecallAt3),
				RecallAt5:          round2(r.AvgRecallAt5),
				RecallAt10:         round2(r.AvgRecallAt10),
				MRR:                round2(r.AvgMRR),
				NDCGAt10:           round2(r.AvgNDCGAt10),
				AvgContextTokens:   round2(r.AvgContextTokens),
				AvgOutputTokens:    round2(r.AvgOutputTokens),
				AvgSearchLatencyMs: round2(r.AvgSearchLatencyMs),
			},
		}
		for _, q := range r.Queries {
			jrr.PerQuery = append(jrr.PerQuery, JSONQueryResult{
				QueryID:              q.QueryID,
				QueryText:            q.QueryText,
				RecallAt3:            round2(q.RecallAt3),
				RecallAt5:            round2(q.RecallAt5),
				RecallAt10:           round2(q.RecallAt10),
				MRR:                  round2(q.MRR),
				NDCGAt10:             round2(q.NDCGAt10),
				ContextTokens:        q.ContextTokens,
				OutputTokensEstimate: q.OutputTokens,
				SearchLatencyMs:      q.SearchLatencyMs,
				TopResults:           q.TopResults,
				Error:                q.Error,
			})
		}
		jr.Runners = append(jr.Runners, jrr)

		// Fill comparison table.
		if r.Status == "ok" {
			reduction := 0.0
			if baselineTokens > 0 {
				reduction = round2((baselineTokens - r.AvgContextTokens) / baselineTokens)
			}
			jr.Comparison.TokenReductionVsBaseline[r.Name] = reduction
			jr.Comparison.RecallAt10[r.Name] = round2(r.AvgRecallAt10)
			jr.Comparison.MRR[r.Name] = round2(r.AvgMRR)
			jr.Comparison.AvgContextTokens[r.Name] = round2(r.AvgContextTokens)
			jr.Comparison.AvgSearchLatencyMs[r.Name] = round2(r.AvgSearchLatencyMs)
		}
	}

	return jr
}

// WriteJSONReport serialises the JSONReport to outputDir/results.json.
func WriteJSONReport(jr JSONReport, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(jr, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outputDir, "results.json"), data, 0o644)
}

// ─── Markdown report ─────────────────────────────────────────────────────────

// WriteMarkdownReport generates a human-readable report.md.
func WriteMarkdownReport(reports []RunnerReport, jr JSONReport, outputDir string) error {
	var sb strings.Builder

	sb.WriteString("# Code Context Retrieval - Comparison Report\n\n")
	sb.WriteString(fmt.Sprintf("Run at: %s  \n", jr.RunAt))
	sb.WriteString(fmt.Sprintf("Fixture: `%s`  \n", jr.FixtureDir))
	sb.WriteString(fmt.Sprintf("Queries: %d\n\n", jr.TotalQueries))

	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Runner | Status | Queries | Recall@3 | Recall@5 | Recall@10 | MRR | nDCG@10 | Avg Context Tokens | Token Reduction | Avg Latency (ms) |\n")
	sb.WriteString("|--------|--------|---------|----------|----------|-----------|-----|---------|-------------------|-----------------|------------------|\n")

	for _, r := range reports {
		if r.Status != "ok" {
			sb.WriteString(fmt.Sprintf("| %s | %s | - | - | - | - | - | - | - | - | - |\n", r.Name, r.Status))
			continue
		}
		reduction := jr.Comparison.TokenReductionVsBaseline[r.Name]
		queriesCell := fmt.Sprintf("%d", r.AggQueryCount)
		if r.FilterNote != "" {
			queriesCell = fmt.Sprintf("%d/%d (%s)", r.AggQueryCount, r.TotalQueryCount, r.FilterNote)
		}
		sb.WriteString(fmt.Sprintf("| **%s** | %s | %s | %.2f | %.2f | %.2f | %.2f | %.2f | %.0f | %.0f%% | %.1f |\n",
			r.Name, r.Status, queriesCell,
			r.AvgRecallAt3, r.AvgRecallAt5, r.AvgRecallAt10,
			r.AvgMRR, r.AvgNDCGAt10,
			r.AvgContextTokens,
			reduction*100,
			r.AvgSearchLatencyMs,
		))
	}

	sb.WriteString("\n## Per-Query Recall@10\n\n")
	// Header
	sb.WriteString("| Query ID | Query |")
	for _, r := range reports {
		if r.Status == "ok" {
			sb.WriteString(fmt.Sprintf(" %s |", r.Name))
		}
	}
	sb.WriteString("\n|----------|-------|")
	for _, r := range reports {
		if r.Status == "ok" {
			_ = r
			sb.WriteString("--------|")
		}
	}
	sb.WriteString("\n")

	// Rows (indexed by query position - all runners have the same query order).
	if len(reports) > 0 {
		baseReport := firstOK(reports)
		if baseReport != nil {
			for qi, qm := range baseReport.Queries {
				label := truncate(qm.QueryText, 40)
				sb.WriteString(fmt.Sprintf("| %s | %s |", qm.QueryID, label))
				for _, r := range reports {
					if r.Status == "ok" && qi < len(r.Queries) {
						sb.WriteString(fmt.Sprintf(" %.2f |", r.Queries[qi].RecallAt10))
					}
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("\n## Index Statistics\n\n")
	sb.WriteString("| Runner | Setup Time (ms) | Index Size |\n")
	sb.WriteString("|--------|-----------------|------------|\n")
	for _, r := range reports {
		size := "-"
		if r.IndexSizeBytes > 0 {
			size = fmt.Sprintf("%d KB", r.IndexSizeBytes/1024)
		}
		sb.WriteString(fmt.Sprintf("| %s | %d | %s |\n", r.Name, r.SetupDurationMs, size))
	}

	sb.WriteString("\n## Methodology\n\n")
	sb.WriteString("- **Token counting**: `len(text)/4` approximation (~15% error for code+English)\n")
	sb.WriteString("- **Recall@K**: fraction of relevant chunks appearing in top-K results\n")
	sb.WriteString("- **MRR**: mean reciprocal rank of the first relevant result across all queries\n")
	sb.WriteString("- **nDCG@10**: normalised discounted cumulative gain with graded relevance (grade 1=relevant, 2=highly relevant)\n")
	sb.WriteString("- **Token reduction**: `(baseline_tokens - runner_tokens) / baseline_tokens`\n")
	sb.WriteString("- **Baseline context** = entire fixture codebase concatenated (~full project)\n")
	sb.WriteString("- **Caveman** output token reduction is estimated (×0.25 per documented compression ratio); retrieval is identical to baseline\n")
	sb.WriteString("- **CCE** skipped if `cce` binary not in PATH\n")

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outputDir, "report.md"), []byte(sb.String()), 0o644)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func firstOK(reports []RunnerReport) *RunnerReport {
	for i := range reports {
		if reports[i].Status == "ok" {
			return &reports[i]
		}
	}
	return nil
}
