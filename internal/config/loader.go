package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const configFileName = ".code-index.yaml"

// Load reads .code-index.yaml from the given root (or any parent directory).
// Missing file is not an error - defaults are returned.
// Environment variables override file values for sensitive fields (API keys).
func Load(root string) (Config, error) {
	cfg := DefaultConfig()

	path := findConfigFile(root)
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, err
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, err
		}
		// If root not set in config, use the directory containing the config file.
		if cfg.Project.Root == "" {
			cfg.Project.Root = filepath.Dir(path)
		}
	}

	if cfg.Project.Root == "" {
		cfg.Project.Root = root
	}

	// Env var override for embedding API key.
	if cfg.Embedding.APIKeyEnv != "" {
		if val := os.Getenv(cfg.Embedding.APIKeyEnv); val != "" {
			// Caller retrieves via cfg.Embedding.APIKeyEnv lookup - stored in env, not config.
			_ = val
		}
	}

	return cfg, nil
}

// findConfigFile walks up the directory tree from root looking for .code-index.yaml.
func findConfigFile(root string) string {
	dir := root
	for {
		candidate := filepath.Join(dir, configFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// APIKey resolves the embedding API key from the environment variable named in cfg.
func APIKey(cfg Config) string {
	if cfg.Embedding.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(cfg.Embedding.APIKeyEnv)
}
