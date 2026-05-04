// Package mcp implements a Model Context Protocol server over stdio.
// The binary is spawned by Claude Code and communicates via JSON-RPC 2.0
// on stdin/stdout. Each session gets a fresh process.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// jsonrpcRequest is an incoming MCP/JSON-RPC 2.0 message.
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// jsonrpcResponse is an outgoing MCP/JSON-RPC 2.0 message.
type jsonrpcResponse struct {
	JSONRPC string  `json:"jsonrpc"`
	ID      any     `json:"id"`
	Result  any     `json:"result,omitempty"`
	Error   *rpcErr `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Server is the MCP stdio server.
type Server struct {
	tools  map[string]ToolHandler
	stdin  io.Reader
	stdout io.Writer
}

// ToolHandler handles a single MCP tool invocation.
type ToolHandler func(params json.RawMessage) (any, error)

// NewServer creates a Server reading from stdin and writing to stdout.
func NewServer() *Server {
	return &Server{
		tools:  make(map[string]ToolHandler),
		stdin:  os.Stdin,
		stdout: os.Stdout,
	}
}

// Register adds a tool handler.
func (s *Server) Register(name string, handler ToolHandler) {
	s.tools[name] = handler
}

// Serve reads JSON-RPC messages in a loop until EOF.
func (s *Server) Serve() error {
	scanner := bufio.NewScanner(s.stdin)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonrpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "parse error")
			continue
		}

		resp := s.dispatch(req)
		s.send(resp)
	}
	return scanner.Err()
}

func (s *Server) dispatch(req jsonrpcRequest) jsonrpcResponse {
	switch req.Method {
	case "initialize":
		return jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"serverInfo": map[string]string{
					"name":    "code-index-for-llms",
					"version": "0.1.0",
				},
				"capabilities": map[string]any{"tools": map[string]any{}},
			},
		}

	case "tools/list":
		return jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"tools": s.toolList()},
		}

	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return errResp(req.ID, -32602, "invalid params")
		}
		handler, ok := s.tools[p.Name]
		if !ok {
			return errResp(req.ID, -32601, fmt.Sprintf("unknown tool: %s", p.Name))
		}
		result, err := handler(p.Arguments)
		if err != nil {
			return errResp(req.ID, -32000, err.Error())
		}
		return jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": mustJSON(result)},
				},
			},
		}

	case "notifications/initialized":
		return jsonrpcResponse{} // No response needed for notifications.

	default:
		return errResp(req.ID, -32601, "method not found: "+req.Method)
	}
}

func (s *Server) toolList() []map[string]any {
	tools := make([]map[string]any, 0, len(s.tools))
	for name := range s.tools {
		tools = append(tools, map[string]any{
			"name":        name,
			"description": toolDescriptions[name],
			"inputSchema": toolSchemas[name],
		})
	}
	return tools
}

func (s *Server) send(resp jsonrpcResponse) {
	if resp.JSONRPC == "" {
		return // Notification - no response.
	}
	data, _ := json.Marshal(resp)
	fmt.Fprintf(s.stdout, "%s\n", data)
}

func (s *Server) sendError(id any, code int, msg string) {
	s.send(errResp(id, code, msg))
}

func errResp(id any, code int, msg string) jsonrpcResponse {
	return jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcErr{Code: code, Message: msg},
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// toolDescriptions maps tool name → human description for tools/list.
var toolDescriptions = map[string]string{
	"index_project":        "Index a codebase for semantic search. Parses all source files, extracts chunks and symbols.",
	"refresh_index":        "Incrementally update the index. Re-indexes only changed files.",
	"get_index_status":     "Return index health, statistics, and detected languages.",
	"search_code":          "Hybrid vector+BM25 search over indexed code chunks.",
	"get_chunk":            "Return the full content of a chunk by ID, with optional surrounding context.",
	"get_file_symbols":     "List all symbols defined or referenced in a file.",
	"find_references":      "Find all definitions and references for a named symbol.",
	"query_resources":      "Query IaC resources (Terraform/HCL) by type, provider, or module.",
	"get_dependency_graph": "Return the dependency graph (nodes + edges) for a chunk or file.",
	"assemble_context":     "Assemble a token-budgeted context block from search + related chunks.",
	"list_languages":       "List available language parsers and detected languages in the project.",
}

// toolSchemas maps tool name → JSON Schema for input validation (simplified).
var toolSchemas = map[string]any{
	"index_project": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"root_path": map[string]string{"type": "string", "description": "Absolute path to the project root"},
		},
		"required": []string{"root_path"},
	},
	"refresh_index": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"root_path":     map[string]string{"type": "string"},
			"changed_files": map[string]any{"type": "array", "items": map[string]string{"type": "string"}},
		},
	},
	"get_index_status": map[string]any{
		"type":       "object",
		"properties": map[string]any{"root_path": map[string]string{"type": "string"}},
	},
	"search_code": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":         map[string]string{"type": "string"},
			"language":      map[string]string{"type": "string"},
			"chunk_types":   map[string]any{"type": "array", "items": map[string]string{"type": "string"}},
			"limit":         map[string]string{"type": "integer"},
			"hybrid_weight": map[string]string{"type": "number", "description": "0.0=BM25 only, 1.0=vector only"},
		},
		"required": []string{"query"},
	},
	"get_chunk": map[string]any{
		"type":       "object",
		"properties": map[string]any{"chunk_id": map[string]string{"type": "string"}, "context_lines": map[string]string{"type": "integer"}},
		"required":   []string{"chunk_id"},
	},
	"get_file_symbols": map[string]any{
		"type":       "object",
		"properties": map[string]any{"file_path": map[string]string{"type": "string"}},
		"required":   []string{"file_path"},
	},
	"find_references": map[string]any{
		"type":       "object",
		"properties": map[string]any{"symbol_name": map[string]string{"type": "string"}, "language": map[string]string{"type": "string"}, "limit": map[string]string{"type": "integer"}},
		"required":   []string{"symbol_name"},
	},
	"query_resources": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"resource_type": map[string]string{"type": "string", "description": "e.g. aws_s3_bucket"},
			"provider":      map[string]string{"type": "string", "description": "e.g. aws"},
			"module_path":   map[string]string{"type": "string"},
			"limit":         map[string]string{"type": "integer"},
		},
	},
	"get_dependency_graph": map[string]any{
		"type":       "object",
		"properties": map[string]any{"chunk_id": map[string]string{"type": "string"}, "file_path": map[string]string{"type": "string"}, "depth": map[string]string{"type": "integer"}},
	},
	"assemble_context": map[string]any{
		"type":       "object",
		"properties": map[string]any{"query": map[string]string{"type": "string"}, "max_tokens": map[string]string{"type": "integer"}},
		"required":   []string{"query"},
	},
	"list_languages": map[string]any{
		"type":       "object",
		"properties": map[string]any{"root_path": map[string]string{"type": "string"}},
	},
}
