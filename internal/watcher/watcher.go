// Package watcher watches a project root for file changes and triggers
// incremental index refreshes automatically.
package watcher

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/code-index-for-llms/code-index/internal/config"
	"github.com/code-index-for-llms/code-index/internal/indexer"
	"github.com/code-index-for-llms/code-index/internal/mcp/handlers"
)

// Watcher watches a project root and triggers a refresh on file changes.
type Watcher struct {
	root  string
	cfg   config.Config
	fsw   *fsnotify.Watcher
	dbDir string // abs path to .code-index/ dir - changes here are ignored
}

// New creates a Watcher for root, registering all non-excluded directories.
func New(root string) (*Watcher, error) {
	cfg, _ := config.Load(root)

	// Derive the directory that holds the SQLite DB so we can ignore its churn.
	dbDir := filepath.Dir(filepath.Join(root, cfg.Storage.Path))

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{root: root, cfg: cfg, fsw: fsw, dbDir: dbDir}
	w.addDirRecursive(root)
	return w, nil
}

// Run blocks, delivering index refreshes on file changes until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) error {
	var timer *time.Timer

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return w.fsw.Close()

		case event, ok := <-w.fsw.Events:
			if !ok {
				return w.fsw.Close()
			}
			if w.shouldIgnore(event.Name) {
				continue
			}
			// Watch newly created directories recursively.
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					w.addDirRecursive(event.Name)
				}
			}
			// Arm or reset debounce timer.
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(2*time.Second, func() {
				if _, err := handlers.RunRefresh(w.root); err != nil {
					fmt.Fprintln(os.Stderr, "[watcher] refresh error:", err)
				}
			})

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return w.fsw.Close()
			}
			fmt.Fprintln(os.Stderr, "[watcher] fsnotify error:", err)
		}
	}
}

// shouldIgnore returns true for paths that must not trigger a refresh.
func (w *Watcher) shouldIgnore(absPath string) bool {
	// Ignore the DB storage directory (changes during refresh itself).
	if strings.HasPrefix(absPath, w.dbDir) {
		return true
	}
	rel, err := filepath.Rel(w.root, absPath)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return indexer.IsExcluded(rel, w.cfg.Index.Exclude)
}

// addDirRecursive registers dir and all non-excluded subdirectories with fsnotify.
func (w *Watcher) addDirRecursive(dir string) {
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(w.root, path)
		rel = filepath.ToSlash(rel)
		if rel != "." && indexer.IsExcluded(rel, w.cfg.Index.Exclude) {
			return filepath.SkipDir
		}
		_ = w.fsw.Add(path)
		return nil
	})
}
