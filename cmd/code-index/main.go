// code-index: semantic code indexing for LLMs
// Usage:
//
//	code-index mcp serve [--root PATH]   - start MCP stdio server (default mode)
//	code-index index [PATH]              - index a project from the CLI
//	code-index search QUERY [--root PATH] - search from the CLI
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/code-index-for-llms/code-index/internal/config"
	"github.com/code-index-for-llms/code-index/internal/mcp"
	"github.com/code-index-for-llms/code-index/internal/mcp/handlers"
	"github.com/code-index-for-llms/code-index/internal/watcher"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "mcp":
		runMCP(os.Args[2:])
	case "index":
		runIndex(os.Args[2:])
	case "search":
		runSearch(os.Args[2:])
	case "version":
		fmt.Println("code-index v0.1.0")
	default:
		printUsage()
		os.Exit(1)
	}
}

func runMCP(args []string) {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	root := fs.String("root", "", "Project root (default: current directory)")
	_ = fs.Parse(args)

	// Sub-command: mcp serve
	if len(fs.Args()) > 0 && fs.Args()[0] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: code-index mcp serve [--root PATH]")
		os.Exit(1)
	}

	if *root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot determine working directory:", err)
			os.Exit(1)
		}
		*root = cwd
	}

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	h := handlers.New(absRoot)

	// Bootstrap: run full index if no DB exists yet.
	cfg, _ := config.Load(absRoot)
	dbPath := filepath.Join(absRoot, cfg.Storage.Path)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "[mcp] no index found, indexing...")
		if _, err := handlers.RunIndexProject(absRoot); err != nil {
			fmt.Fprintln(os.Stderr, "[mcp] initial index failed:", err)
		}
	}

	// Start background file watcher.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if w, err := watcher.New(absRoot); err != nil {
		fmt.Fprintln(os.Stderr, "[mcp] watcher init failed (continuing without):", err)
	} else {
		go func() {
			if err := w.Run(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "[mcp] watcher stopped:", err)
			}
		}()
	}

	srv := mcp.NewServer()

	srv.Register("index_project", h.IndexProject)
	srv.Register("refresh_index", h.RefreshIndex)
	srv.Register("get_index_status", h.GetIndexStatus)
	srv.Register("search_code", h.SearchCode)
	srv.Register("get_chunk", h.GetChunk)
	srv.Register("get_file_symbols", h.GetFileSymbols)
	srv.Register("find_references", h.FindReferences)
	srv.Register("query_resources", h.QueryResources)
	srv.Register("get_dependency_graph", h.GetDependencyGraph)
	srv.Register("assemble_context", h.AssembleContext)
	srv.Register("list_languages", h.ListLanguages)

	if err := srv.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, "mcp server error:", err)
		os.Exit(1)
	}
}

func runIndex(args []string) {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	_ = fs.Parse(args)

	root := "."
	if len(fs.Args()) > 0 {
		root = fs.Args()[0]
	}

	absRoot, _ := filepath.Abs(root)
	h := handlers.New(absRoot)

	params, _ := json.Marshal(map[string]string{"root_path": absRoot})
	result, err := h.IndexProject(params)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	b, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(b))
}

func runSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	root := fs.String("root", ".", "Project root")
	limit := fs.Int("limit", 5, "Max results")
	_ = fs.Parse(args)

	if len(fs.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "usage: code-index search QUERY [--root PATH] [--limit N]")
		os.Exit(1)
	}

	absRoot, _ := filepath.Abs(*root)
	h := handlers.New(absRoot)

	params, _ := json.Marshal(map[string]any{
		"query":     fs.Args()[0],
		"root_path": absRoot,
		"limit":     *limit,
	})
	result, err := h.SearchCode(params)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	b, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(b))
}

func printUsage() {
	fmt.Println(`code-index - semantic code indexing for LLMs

Usage:
  code-index mcp serve [--root PATH]        Start MCP stdio server
  code-index index [PATH]                   Index a project
  code-index search QUERY [--root PATH]     Search indexed code
  code-index version                        Print version`)
}
