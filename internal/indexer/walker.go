package indexer

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/code-index-for-llms/code-index/internal/config"
)

// FileEntry is a discovered file ready for indexing.
type FileEntry struct {
	AbsPath  string
	RelPath  string
	Language string
	SizeBytes int64
	ContentHash string
	Content  []byte
}

// Walk traverses root and returns all indexable files matching cfg.
func Walk(root string, cfg config.Config) ([]FileEntry, error) {
	var entries []FileEntry

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable paths
		}

		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if IsExcluded(rel, cfg.Index.Exclude) {
				return filepath.SkipDir
			}
			return nil
		}

		if IsExcluded(rel, cfg.Index.Exclude) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		maxBytes := int64(cfg.Index.MaxFileSizeKB) * 1024
		if info.Size() > maxBytes {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Skip binary files (heuristic: null bytes in first 8KB).
		if isBinary(content) {
			return nil
		}

		h := sha256.Sum256(content)
		entries = append(entries, FileEntry{
			AbsPath:     path,
			RelPath:     rel,
			SizeBytes:   info.Size(),
			ContentHash: fmt.Sprintf("%x", h),
			Content:     content,
		})
		return nil
	})

	return entries, err
}

// IsExcluded returns true if relPath matches any exclusion pattern.
func IsExcluded(relPath string, patterns []string) bool {
	base := filepath.Base(relPath)
	for _, pat := range patterns {
		// Directory name match
		if matched, _ := filepath.Match(pat, base); matched {
			return true
		}
		// Full relative path match
		if matched, _ := filepath.Match(pat, relPath); matched {
			return true
		}
		// Prefix match for directory exclusions (e.g. ".git" excludes ".git/config")
		if strings.HasPrefix(relPath, pat+"/") || relPath == pat {
			return true
		}
	}
	return false
}

// isBinary detects binary files by scanning for null bytes.
func isBinary(content []byte) bool {
	check := content
	if len(check) > 8192 {
		check = check[:8192]
	}
	for _, b := range check {
		if b == 0 {
			return true
		}
	}
	return false
}
