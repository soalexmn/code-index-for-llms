package runners

// TokenCounter estimates token counts for text strings.
// The default implementation uses len(text)/4, which is within ~15% of the
// actual cl100k_base tiktoken count for typical code and English text.
// Zero external dependencies required.
type TokenCounter interface {
	Count(text string) int
}

// SimpleTokenCounter approximates tokens as len(text)/4.
// Accurate to within ~15% for code+English. Zero dependencies.
type SimpleTokenCounter struct{}

// Count returns an approximate token count for the given text.
func (SimpleTokenCounter) Count(text string) int {
	n := len(text) / 4
	if n == 0 && len(text) > 0 {
		return 1
	}
	return n
}

// DefaultTokenCounter is the global token counter used by all runners.
var DefaultTokenCounter TokenCounter = SimpleTokenCounter{}
