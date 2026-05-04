package golang

import (
	"testing"

	"github.com/code-index-for-llms/code-index/pkg/types"
)

func TestParse_FuncsAndTypes(t *testing.T) {
	src := `package main

import "fmt"

type Server struct {
	host string
	port int
}

type Handler interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

func NewServer(host string, port int) *Server {
	return &Server{host: host, port: port}
}

func (s *Server) Start() error {
	fmt.Println("starting")
	return nil
}

func (s *Server) Stop() {
}

func standalone() {
	fmt.Println("hello")
}
`
	p := New()
	chunks, err := p.Parse("server.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}

	want := []struct {
		name string
		kind types.ChunkType
	}{
		{"Server", types.ChunkTypeClass},
		{"Handler", types.ChunkTypeInterface},
		{"NewServer", types.ChunkTypeFunction},
		{"Server.Start", types.ChunkTypeMethod},
		{"Server.Stop", types.ChunkTypeMethod},
		{"standalone", types.ChunkTypeFunction},
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

func TestParse_ReceiverPointer(t *testing.T) {
	src := `package foo

type MyType struct{}

func (m *MyType) DoThing() error { return nil }
func (m MyType) ReadThing() string { return "" }
`
	p := New()
	chunks, _ := p.Parse("t.go", []byte(src))
	names := chunkNames(chunks)

	found := map[string]bool{}
	for _, c := range chunks {
		found[c.Name] = true
	}
	if !found["MyType.DoThing"] {
		t.Errorf("missing MyType.DoThing in %v", names)
	}
	if !found["MyType.ReadThing"] {
		t.Errorf("missing MyType.ReadThing in %v", names)
	}
}

func TestParse_GenericReceiver(t *testing.T) {
	src := `package foo

type Stack[T any] struct{ items []T }

func (s *Stack[T]) Push(item T) { s.items = append(s.items, item) }
`
	p := New()
	chunks, _ := p.Parse("t.go", []byte(src))
	found := false
	for _, c := range chunks {
		if c.Name == "Stack.Push" && c.ChunkType == types.ChunkTypeMethod {
			found = true
		}
	}
	if !found {
		t.Errorf("Stack.Push not found in %v", chunkNames(chunks))
	}
}

func TestParse_BlockEnd(t *testing.T) {
	src := `package foo

func Alpha() {
	x := 1
	_ = x
}

func Beta() {
	y := 2
	_ = y
}
`
	p := New()
	chunks, _ := p.Parse("t.go", []byte(src))
	for _, c := range chunks {
		if c.Name == "Alpha" {
			if c.StartLine != 3 {
				t.Errorf("Alpha.StartLine = %d, want 3", c.StartLine)
			}
			if c.EndLine != 6 {
				t.Errorf("Alpha.EndLine = %d, want 6", c.EndLine)
			}
		}
	}
}

func TestParse_EmptyFile(t *testing.T) {
	p := New()
	chunks, err := p.Parse("empty.go", []byte("package foo\n"))
	if err != nil {
		t.Fatal(err)
	}
	// No functions/types → FILE fallback.
	if len(chunks) != 1 || chunks[0].ChunkType != types.ChunkTypeFile {
		t.Errorf("empty Go file: got %v, want single FILE chunk", chunkNames(chunks))
	}
}

func TestExtractSymbols(t *testing.T) {
	src := `package foo
func Foo() {}
type Bar struct{}
`
	p := New()
	chunks, _ := p.Parse("t.go", []byte(src))
	syms, err := p.ExtractSymbols(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 2 {
		t.Errorf("want 2 symbols, got %d", len(syms))
	}
}

func chunkNames(chunks []types.Chunk) []string {
	names := make([]string, len(chunks))
	for i, c := range chunks {
		names[i] = string(c.ChunkType) + ":" + c.Name
	}
	return names
}
