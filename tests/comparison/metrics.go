package comparison

import "github.com/code-index-for-llms/code-index/pkg/types"

// ResultKey constructs the canonical relevance key for a SearchResult.
// It matches the format used in queries.json.
func ResultKey(r types.SearchResult) string {
	return r.Chunk.FilePath + "::" + r.Chunk.Name
}

// BuildRelevanceSet converts the slice of relevant chunk annotations from
// queries.json into a RelevanceSet map with lower-cased file paths.
func BuildRelevanceSet(chunks []RelevantChunk) RelevanceSet {
	rs := make(RelevanceSet, len(chunks))
	for _, c := range chunks {
		// Lower-case the file path for case-insensitive matching.
		fp := ""
		for _, ch := range c.FilePath {
			if ch >= 'A' && ch <= 'Z' {
				fp += string(rune(ch + 32))
			} else {
				fp += string(ch)
			}
		}
		rs[fp+"::"+c.ChunkName] = c.Grade
	}
	return rs
}
