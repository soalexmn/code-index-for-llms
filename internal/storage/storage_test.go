package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/code-index-for-llms/code-index/pkg/types"
)

func openTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestMigrate_Idempotent(t *testing.T) {
	store := openTestStore(t)
	// Running Migrate twice must not error (idempotency check).
	if err := store.Migrate(); err != nil {
		t.Errorf("second Migrate: %v", err)
	}
}

func TestUpsertProject_GetProject(t *testing.T) {
	store := openTestStore(t)

	proj := types.Project{
		ID:            "proj1",
		RootPath:      "/tmp/testproj",
		Name:          "testproj",
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC().Truncate(time.Second),
		LastIndexedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := store.UpsertProject(proj); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetProject("/tmp/testproj")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != proj.ID {
		t.Errorf("ID = %q, want %q", got.ID, proj.ID)
	}
	if got.Name != proj.Name {
		t.Errorf("Name = %q, want %q", got.Name, proj.Name)
	}
}

func TestUpsertFile_ListFiles(t *testing.T) {
	store := openTestStore(t)
	insertProject(t, store, "proj1", "/tmp/p")

	files := []types.File{
		{ID: "f1", ProjectID: "proj1", RelativePath: "a.go", ContentHash: "abc", LastModified: time.Now(), IndexedAt: time.Now()},
		{ID: "f2", ProjectID: "proj1", RelativePath: "b.go", ContentHash: "def", LastModified: time.Now(), IndexedAt: time.Now()},
	}
	for _, f := range files {
		if err := store.UpsertFile(f); err != nil {
			t.Fatalf("UpsertFile: %v", err)
		}
	}

	listed, err := store.ListFiles("proj1")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Errorf("ListFiles: got %d, want 2", len(listed))
	}
}

func TestUpsertChunks_Search(t *testing.T) {
	store := openTestStore(t)
	insertProject(t, store, "proj1", "/tmp/p")
	insertFile(t, store, "f1", "proj1", "main.go")

	chunks := []types.Chunk{
		{
			ID: "c1", FileID: "f1", ProjectID: "proj1",
			FilePath: "main.go", Language: "go",
			ChunkType: types.ChunkTypeFunction, Name: "ParseConfig",
			Content:     "func ParseConfig(path string) (*Config, error) { return nil, nil }",
			StartLine:   1, EndLine: 1,
			ContentHash: "h1", IndexedAt: time.Now(),
		},
		{
			ID: "c2", FileID: "f1", ProjectID: "proj1",
			FilePath: "main.go", Language: "go",
			ChunkType: types.ChunkTypeFunction, Name: "RunServer",
			Content:     "func RunServer(addr string) error { return nil }",
			StartLine:   3, EndLine: 3,
			ContentHash: "h2", IndexedAt: time.Now(),
		},
	}
	if err := store.UpsertChunks(chunks); err != nil {
		t.Fatal(err)
	}

	results, err := store.Search(SearchRequest{
		ProjectID: "proj1",
		Query:     "ParseConfig",
		Limit:     5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected search results, got none")
	}
	if results[0].Chunk.Name != "ParseConfig" {
		t.Errorf("top result = %q, want ParseConfig", results[0].Chunk.Name)
	}
}

func TestGetChunk(t *testing.T) {
	store := openTestStore(t)
	insertProject(t, store, "proj1", "/tmp/p")
	insertFile(t, store, "f1", "proj1", "a.go")

	chunk := types.Chunk{
		ID: "c1", FileID: "f1", ProjectID: "proj1",
		FilePath: "a.go", Language: "go",
		ChunkType: types.ChunkTypeFunction, Name: "Foo",
		Content: "func Foo() {}", StartLine: 1, EndLine: 1,
		ContentHash: "hh", IndexedAt: time.Now(),
	}
	if err := store.UpsertChunks([]types.Chunk{chunk}); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetChunk("c1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Foo" {
		t.Errorf("Name = %q, want Foo", got.Name)
	}
	if got.Language != "go" {
		t.Errorf("Language = %q, want go", got.Language)
	}
	if got.FilePath != "a.go" {
		t.Errorf("FilePath = %q, want a.go", got.FilePath)
	}
}

func TestDeleteChunksByFile(t *testing.T) {
	store := openTestStore(t)
	insertProject(t, store, "proj1", "/tmp/p")
	insertFile(t, store, "f1", "proj1", "a.go")

	chunks := []types.Chunk{
		{ID: "c1", FileID: "f1", ProjectID: "proj1", FilePath: "a.go",
			ChunkType: types.ChunkTypeFunction, Name: "A",
			Content: "func A() {}", StartLine: 1, EndLine: 1, ContentHash: "h1", IndexedAt: time.Now()},
	}
	_ = store.UpsertChunks(chunks)
	_ = store.DeleteChunksByFile("f1")

	_, err := store.GetChunk("c1")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestUpsertSymbols_GetFileSymbols(t *testing.T) {
	store := openTestStore(t)
	insertProject(t, store, "proj1", "/tmp/p")
	insertFile(t, store, "f1", "proj1", "a.go")

	syms := []types.Symbol{
		{ID: "s1", ChunkID: "c1", FileID: "f1", Name: "Foo", Kind: types.SymbolKindDefinition, Line: 1, Language: "go"},
		{ID: "s2", ChunkID: "c1", FileID: "f1", Name: "Bar", Kind: types.SymbolKindReference, Line: 5, Language: "go"},
	}
	if err := store.UpsertSymbols(syms); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetFileSymbols("f1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("GetFileSymbols: got %d, want 2", len(got))
	}
}

func TestFindReferences(t *testing.T) {
	store := openTestStore(t)
	insertProject(t, store, "proj1", "/tmp/p")
	insertFile(t, store, "f1", "proj1", "a.go")

	syms := []types.Symbol{
		{ID: "s1", ChunkID: "c1", FileID: "f1", Name: "MyFunc", Kind: types.SymbolKindDefinition, Line: 1, Language: "go"},
		{ID: "s2", ChunkID: "c2", FileID: "f1", Name: "MyFunc", Kind: types.SymbolKindReference, Line: 10, Language: "go"},
	}
	_ = store.UpsertSymbols(syms)

	refs, err := store.FindReferences("proj1", "MyFunc")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Errorf("FindReferences: got %d, want 2", len(refs))
	}
}

func TestIndexStatus(t *testing.T) {
	store := openTestStore(t)
	insertProject(t, store, "proj1", "/tmp/p")
	insertFile(t, store, "f1", "proj1", "a.go")

	status, err := store.IndexStatus("proj1")
	if err != nil {
		t.Fatal(err)
	}
	if status.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1", status.TotalFiles)
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func insertProject(t *testing.T, s *SQLiteStore, id, root string) {
	t.Helper()
	err := s.UpsertProject(types.Project{
		ID: id, RootPath: root, Name: id, SchemaVersion: 1,
		CreatedAt: time.Now(), LastIndexedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("insertProject: %v", err)
	}
}

func insertFile(t *testing.T, s *SQLiteStore, fileID, projID, relPath string) {
	t.Helper()
	_ = os.MkdirAll(filepath.Join(t.TempDir(), filepath.Dir(relPath)), 0o755)
	err := s.UpsertFile(types.File{
		ID: fileID, ProjectID: projID,
		RelativePath: relPath, AbsolutePath: "/tmp/" + relPath,
		ContentHash: "x", LastModified: time.Now(), IndexedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("insertFile: %v", err)
	}
}
