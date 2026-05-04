package python

import (
	"testing"

	"github.com/code-index-for-llms/code-index/pkg/types"
)

func TestParse_FunctionsAndClasses(t *testing.T) {
	src := `import os

class MyClass:
    """A simple class."""

    def __init__(self, name: str):
        self.name = name

    @staticmethod
    def create() -> "MyClass":
        return MyClass("default")

    async def fetch(self, url: str) -> str:
        pass

def standalone(x: int) -> int:
    return x * 2

async def async_top():
    pass
`
	p := New()
	chunks, err := p.Parse("test.py", []byte(src))
	if err != nil {
		t.Fatal(err)
	}

	want := []struct {
		name string
		kind types.ChunkType
	}{
		{"MyClass", types.ChunkTypeClass},
		{"MyClass.__init__", types.ChunkTypeMethod},
		{"MyClass.create", types.ChunkTypeMethod},
		{"MyClass.fetch", types.ChunkTypeMethod},
		{"standalone", types.ChunkTypeFunction},
		{"async_top", types.ChunkTypeFunction},
	}

	if len(chunks) != len(want) {
		t.Fatalf("got %d chunks, want %d\nchunks: %v", len(chunks), len(want), chunkNames(chunks))
	}
	for i, w := range want {
		if chunks[i].Name != w.name {
			t.Errorf("chunk[%d].Name = %q, want %q", i, chunks[i].Name, w.name)
		}
		if chunks[i].ChunkType != w.kind {
			t.Errorf("chunk[%d].ChunkType = %q, want %q", i, chunks[i].ChunkType, w.kind)
		}
	}
}

func TestParse_DecoratorStart(t *testing.T) {
	src := `class Foo:
    @property
    def bar(self):
        return self._bar

    @bar.setter
    def bar(self, val):
        self._bar = val
`
	p := New()
	chunks, err := p.Parse("test.py", []byte(src))
	if err != nil {
		t.Fatal(err)
	}

	// Expect class + 2 methods; decorator lines included in method start.
	if len(chunks) < 3 {
		t.Fatalf("want ≥3 chunks, got %d: %v", len(chunks), chunkNames(chunks))
	}
	// First bar method should start at the @property line (line 2), not line 3.
	for _, c := range chunks {
		if c.Name == "Foo.bar" {
			if c.StartLine != 2 {
				t.Errorf("first Foo.bar StartLine = %d, want 2 (decorator line)", c.StartLine)
			}
			break // Only check first occurrence; second setter legitimately starts at line 6.
		}
	}
}

func TestParse_EmptyFile(t *testing.T) {
	p := New()
	chunks, err := p.Parse("empty.py", []byte(""))
	if err != nil {
		t.Fatal(err)
	}
	// Falls back to FILE chunk.
	if len(chunks) != 1 || chunks[0].ChunkType != types.ChunkTypeFile {
		t.Errorf("empty file: got %v, want single FILE chunk", chunkNames(chunks))
	}
}

func TestParse_FilePath(t *testing.T) {
	src := `def foo(): pass`
	p := New()
	chunks, _ := p.Parse("pkg/module.py", []byte(src))
	for _, c := range chunks {
		if c.FilePath != "pkg/module.py" {
			t.Errorf("FilePath = %q, want %q", c.FilePath, "pkg/module.py")
		}
	}
}

func TestParse_LineNumbers(t *testing.T) {
	src := `class A:
    def m(self):
        pass

def top():
    pass
`
	p := New()
	chunks, _ := p.Parse("t.py", []byte(src))
	for _, c := range chunks {
		switch c.Name {
		case "A":
			if c.StartLine != 1 {
				t.Errorf("A.StartLine = %d, want 1", c.StartLine)
			}
		case "A.m":
			if c.StartLine != 2 {
				t.Errorf("A.m.StartLine = %d, want 2", c.StartLine)
			}
		case "top":
			if c.StartLine != 5 {
				t.Errorf("top.StartLine = %d, want 5", c.StartLine)
			}
		}
	}
}

func TestExtractSymbols(t *testing.T) {
	src := `def foo(): pass
class Bar: pass
`
	p := New()
	chunks, _ := p.Parse("t.py", []byte(src))
	syms, err := p.ExtractSymbols(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 2 {
		t.Errorf("want 2 symbols, got %d", len(syms))
	}
	for _, s := range syms {
		if s.Kind != types.SymbolKindDefinition {
			t.Errorf("symbol %q kind = %q, want DEFINITION", s.Name, s.Kind)
		}
	}
}

func chunkNames(chunks []types.Chunk) []string {
	names := make([]string, len(chunks))
	for i, c := range chunks {
		names[i] = string(c.ChunkType) + ":" + c.Name
	}
	return names
}
