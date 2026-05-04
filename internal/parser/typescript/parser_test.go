package typescript

import (
	"testing"

	"github.com/code-index-for-llms/code-index/pkg/types"
)

func TestParse_ClassesAndFunctions(t *testing.T) {
	src := `import { Injectable } from '@angular/core';

export interface UserRepository {
  findById(id: string): Promise<User>;
  save(user: User): Promise<void>;
}

@Injectable()
export class UserService {
  constructor(private repo: UserRepository) {}

  async getUser(id: string): Promise<User> {
    return this.repo.findById(id);
  }

  async createUser(data: Partial<User>): Promise<User> {
    const user = new User(data);
    return this.repo.save(user);
  }
}

export async function bootstrap(port: number): Promise<void> {
  console.log('starting on', port);
}

const helper = (x: number) => x * 2;
`
	p := New()
	chunks, err := p.Parse("service.ts", []byte(src))
	if err != nil {
		t.Fatal(err)
	}

	wantNames := map[string]types.ChunkType{
		"UserRepository":         types.ChunkTypeInterface,
		"UserService":            types.ChunkTypeClass,
		"UserService.getUser":    types.ChunkTypeMethod,
		"UserService.createUser": types.ChunkTypeMethod,
		"bootstrap":              types.ChunkTypeFunction,
	}

	found := map[string]types.ChunkType{}
	for _, c := range chunks {
		found[c.Name] = c.ChunkType
	}

	for name, kind := range wantNames {
		got, ok := found[name]
		if !ok {
			t.Errorf("missing chunk %q (have: %v)", name, chunkNames(chunks))
			continue
		}
		if got != kind {
			t.Errorf("%q.ChunkType = %q, want %q", name, got, kind)
		}
	}
}

func TestParse_JavaScript(t *testing.T) {
	src := `class EventEmitter {
  constructor() {
    this.listeners = {};
  }

  on(event, fn) {
    this.listeners[event] = fn;
  }

  emit(event, ...args) {
    if (this.listeners[event]) {
      this.listeners[event](...args);
    }
  }
}

function createEmitter() {
  return new EventEmitter();
}
`
	p := New()
	chunks, err := p.Parse("events.js", []byte(src))
	if err != nil {
		t.Fatal(err)
	}

	// Verify language is "javascript" for .js files.
	for _, c := range chunks {
		if c.Language != "javascript" {
			t.Errorf("chunk %q language = %q, want javascript", c.Name, c.Language)
		}
	}

	found := map[string]bool{}
	for _, c := range chunks {
		found[c.Name] = true
	}
	if !found["EventEmitter"] {
		t.Errorf("missing EventEmitter class, have: %v", chunkNames(chunks))
	}
	if !found["createEmitter"] {
		t.Errorf("missing createEmitter fn, have: %v", chunkNames(chunks))
	}
}

func TestParse_DecoratorClass(t *testing.T) {
	src := `@Component({
  selector: 'app-root',
})
export class AppComponent {
  title = 'app';
}
`
	p := New()
	chunks, err := p.Parse("app.component.ts", []byte(src))
	if err != nil {
		t.Fatal(err)
	}

	var cls *types.Chunk
	for i := range chunks {
		if chunks[i].Name == "AppComponent" {
			cls = &chunks[i]
		}
	}
	if cls == nil {
		t.Fatalf("AppComponent not found in %v", chunkNames(chunks))
	}
	// Should start at the @Component decorator line.
	if cls.StartLine != 1 {
		t.Errorf("AppComponent.StartLine = %d, want 1 (decorator)", cls.StartLine)
	}
}

func TestParse_EmptyFile(t *testing.T) {
	p := New()
	chunks, err := p.Parse("empty.ts", []byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 || chunks[0].ChunkType != types.ChunkTypeFile {
		t.Errorf("empty file: got %v, want single FILE chunk", chunkNames(chunks))
	}
}

func TestExtractSymbols(t *testing.T) {
	src := `export class Foo {}
export function bar() {}
export interface Baz {}
`
	p := New()
	chunks, _ := p.Parse("t.ts", []byte(src))
	syms, err := p.ExtractSymbols(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 3 {
		t.Errorf("want 3 symbols, got %d", len(syms))
	}
}

func chunkNames(chunks []types.Chunk) []string {
	names := make([]string, len(chunks))
	for i, c := range chunks {
		names[i] = string(c.ChunkType) + ":" + c.Name
	}
	return names
}
