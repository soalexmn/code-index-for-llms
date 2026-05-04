package config

// Config is the full project configuration, loaded from .code-index.yaml.
type Config struct {
	Project   ProjectConfig   `yaml:"project"`
	Index     IndexConfig     `yaml:"index"`
	Embedding EmbeddingConfig `yaml:"embedding"`
	MCP       MCPConfig       `yaml:"mcp"`
	Storage   StorageConfig   `yaml:"storage"`
}

type ProjectConfig struct {
	Name string `yaml:"name"`
	Root string `yaml:"root"` // Defaults to directory containing .code-index.yaml
}

type IndexConfig struct {
	Include      []string `yaml:"include"`       // Glob patterns; default ["**/*"]
	Exclude      []string `yaml:"exclude"`       // Default: common noise dirs
	MaxFileSizeKB int     `yaml:"max_file_size_kb"` // Default: 500
	Languages    []string `yaml:"languages"`     // Restrict languages; empty = auto-detect
}

type EmbeddingConfig struct {
	Provider  string `yaml:"provider"`    // "openai", "ollama", "none" (default)
	Model     string `yaml:"model"`
	APIKeyEnv string `yaml:"api_key_env"` // Env var name holding the key
	BaseURL   string `yaml:"base_url"`    // For Ollama or self-hosted
	Dimensions int   `yaml:"dimensions"`
}

type MCPConfig struct {
	MaxResults   int `yaml:"max_results"`    // Default: 10
	ContextLines int `yaml:"context_lines"`  // Lines of surrounding context; default: 3
}

type StorageConfig struct {
	Path string `yaml:"path"` // Default: .code-index/index.db
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Index: IndexConfig{
			Include: []string{"**/*"},
			Exclude: []string{
				".git", ".hg", ".svn",
				"node_modules", "vendor", ".venv", "__pycache__",
				".terraform", ".terraform.lock.hcl",
				"dist", "build", "out", "target",
				"*.min.js", "*.min.css",
				".code-index",
			},
			MaxFileSizeKB: 500,
		},
		Embedding: EmbeddingConfig{
			Provider: "none",
		},
		MCP: MCPConfig{
			MaxResults:   10,
			ContextLines: 3,
		},
		Storage: StorageConfig{
			Path: ".code-index/index.db",
		},
	}
}
