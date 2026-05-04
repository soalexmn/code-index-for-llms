package parser

import (
	"path/filepath"
	"strings"
	"sync"
)

// Registry dispatches source files to the correct LanguageParser.
// Parsers are registered at compile time; there is no dynamic loading.
type Registry struct {
	mu       sync.RWMutex
	parsers  map[string]LanguageParser   // keyed by language name
	byExt    map[string][]LanguageParser // keyed by extension (multiple parsers may claim same ext)
	fallback LanguageParser              // generic line-based chunker, always last resort
}

// NewRegistry returns an empty Registry. Use Register to add parsers.
func NewRegistry() *Registry {
	return &Registry{
		parsers: make(map[string]LanguageParser),
		byExt:   make(map[string][]LanguageParser),
	}
}

// SetFallback sets the parser used when no other parser matches.
func (r *Registry) SetFallback(p LanguageParser) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallback = p
}

// Register adds a parser to the registry. Panics on duplicate language name.
func (r *Registry) Register(p LanguageParser) {
	r.mu.Lock()
	defer r.mu.Unlock()

	lang := p.Language()
	if _, exists := r.parsers[lang]; exists {
		panic("parser already registered for language: " + lang)
	}
	r.parsers[lang] = p

	for _, ext := range p.Extensions() {
		ext = strings.ToLower(ext)
		r.byExt[ext] = append(r.byExt[ext], p)
	}
}

// Detect returns the best parser for the given file, falling back to the
// generic parser if no language-specific parser matches.
func (r *Registry) Detect(filePath string, content []byte) LanguageParser {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ext := strings.ToLower(filepath.Ext(filePath))
	candidates := r.byExt[ext]

	// Single candidate - use it directly without CanParse overhead.
	if len(candidates) == 1 {
		return candidates[0]
	}

	// Multiple candidates share the extension; let CanParse break the tie.
	for _, p := range candidates {
		if p.CanParse(filePath, content) {
			return p
		}
	}

	return r.fallback
}

// GetByLanguage returns the parser for an explicit language name.
func (r *Registry) GetByLanguage(language string) (LanguageParser, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.parsers[language]
	return p, ok
}

// ListLanguages returns all registered language names.
func (r *Registry) ListLanguages() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	langs := make([]string, 0, len(r.parsers))
	for lang := range r.parsers {
		langs = append(langs, lang)
	}
	return langs
}

// GetFallback returns the fallback parser (may be nil if not set).
func (r *Registry) GetFallback() LanguageParser {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.fallback
}

// DefaultRegistry is the global registry pre-populated with all built-in parsers.
// Import _ "github.com/code-index-for-llms/code-index/internal/parser/terraform" etc.
// from main.go to trigger init() registration.
var DefaultRegistry = NewRegistry()
