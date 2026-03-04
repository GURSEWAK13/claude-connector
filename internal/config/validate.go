package config

import "fmt"

func Validate(cfg *Config) error {
	if cfg.Proxy.Port <= 0 || cfg.Proxy.Port > 65535 {
		return fmt.Errorf("proxy.port must be 1-65535, got %d", cfg.Proxy.Port)
	}
	if cfg.Peer.Port <= 0 || cfg.Peer.Port > 65535 {
		return fmt.Errorf("peer.port must be 1-65535, got %d", cfg.Peer.Port)
	}
	if cfg.Web.Port <= 0 || cfg.Web.Port > 65535 {
		return fmt.Errorf("web.port must be 1-65535, got %d", cfg.Web.Port)
	}
	if cfg.Proxy.Port == cfg.Peer.Port || cfg.Proxy.Port == cfg.Web.Port || cfg.Peer.Port == cfg.Web.Port {
		return fmt.Errorf("proxy, peer, and web ports must be different")
	}

	for i, s := range cfg.Sessions.Sessions {
		if s.ID == "" {
			return fmt.Errorf("sessions[%d].id is required", i)
		}
		if s.Type != "claude_web" && s.Type != "anthropic_api" {
			return fmt.Errorf("sessions[%d].type must be 'claude_web' or 'anthropic_api', got %q", i, s.Type)
		}
		if s.SessionKey == "" {
			return fmt.Errorf("sessions[%d].session_key is required", i)
		}
	}

	return nil
}
