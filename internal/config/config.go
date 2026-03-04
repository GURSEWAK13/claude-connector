package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	NodeID   string `toml:"node_id"`
	NodeName string `toml:"node_name"`

	Proxy    ProxyConfig    `toml:"proxy"`
	Peer     PeerConfig     `toml:"peer"`
	Web      WebConfig      `toml:"web"`
	Sessions SessionsConfig `toml:"sessions"`
	Fallback FallbackConfig `toml:"fallback"`
}

type ProxyConfig struct {
	Port   int    `toml:"port"`
	APIKey string `toml:"api_key"`
}

type PeerConfig struct {
	Port         int    `toml:"port"`
	SharedSecret string `toml:"shared_secret"`
}

type WebConfig struct {
	Port        int    `toml:"port"`
	BindAddress string `toml:"bind_address"`
}

type SessionsConfig struct {
	Sessions []SessionEntry `toml:"session"`
}

type SessionEntry struct {
	ID         string `toml:"id"`
	Type       string `toml:"type"` // "claude_web" | "anthropic_api"
	SessionKey string `toml:"session_key"`
	Enabled    bool   `toml:"enabled"`
}

type FallbackConfig struct {
	OllamaEnabled      bool   `toml:"ollama_enabled"`
	OllamaURL          string `toml:"ollama_url"`
	OllamaDefaultModel string `toml:"ollama_default_model"`
	LMStudioEnabled    bool   `toml:"lmstudio_enabled"`
	LMStudioURL        string `toml:"lmstudio_url"`
}

func DefaultConfig() *Config {
	hostname, _ := os.Hostname()
	return &Config{
		NodeID:   uuid.New().String(),
		NodeName: hostname,
		Proxy: ProxyConfig{
			Port: 8765,
		},
		Peer: PeerConfig{
			Port:         8767,
			SharedSecret: "",
		},
		Web: WebConfig{
			Port:        8766,
			BindAddress: "127.0.0.1",
		},
		Fallback: FallbackConfig{
			OllamaEnabled:      true,
			OllamaURL:          "http://localhost:11434",
			OllamaDefaultModel: "llama3.2:latest",
			LMStudioEnabled:    true,
			LMStudioURL:        "http://localhost:1234",
		},
	}
}

func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "claude-connector", "config.toml")
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = ConfigPath()
	}

	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Save default config
			if saveErr := Save(cfg, path); saveErr != nil {
				// Non-fatal: just return the default
				return cfg, nil
			}
			fmt.Printf("Created default config at %s\n", path)
			return cfg, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	return cfg, nil
}

func Save(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}
