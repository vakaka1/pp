package ppweb

import (
	"strings"
	"testing"

	"github.com/vakaka1/pp/internal/config"
)

func TestShouldManageNginx(t *testing.T) {
	server := &Server{}

	loopback := &Connection{
		Enabled: true,
		Listen:  "127.0.0.1:8081",
		TLS:     &config.TLSConfig{Enabled: true, CertFile: "/tmp/cert.pem", KeyFile: "/tmp/key.pem"},
		Settings: map[string]any{
			"domain": "loopback.example.com",
		},
	}
	if !server.shouldManageNginx(loopback) {
		t.Fatalf("expected loopback TLS connection to be published through nginx")
	}

	direct := &Connection{
		Enabled: true,
		Listen:  "0.0.0.0:443",
		TLS:     &config.TLSConfig{Enabled: true, CertFile: "/tmp/cert.pem", KeyFile: "/tmp/key.pem"},
		Settings: map[string]any{
			"domain": "direct.example.com",
		},
	}
	if server.shouldManageNginx(direct) {
		t.Fatalf("expected direct :443 TLS connection to stay direct")
	}

	noTLS := &Connection{
		Enabled: true,
		Listen:  "127.0.0.1:8082",
	}
	if server.shouldManageNginx(noTLS) {
		t.Fatalf("expected connection without HTTPS to skip nginx publishing")
	}
}

func TestBuildNginxConfigUsesConnectionListenAndPath(t *testing.T) {
	server := &Server{}
	connection := &Connection{
		Tag:    "blog-1",
		Listen: "127.0.0.1:8085",
		TLS:    &config.TLSConfig{Enabled: true, CertFile: "/tmp/cert.pem", KeyFile: "/tmp/key.pem"},
		Settings: map[string]any{
			"domain":    "blog.example.com",
			"grpc_path": "/grpc-custom",
		},
	}

	configText, err := server.buildNginxConfig(connection)
	if err != nil {
		t.Fatalf("buildNginxConfig() error = %v", err)
	}
	if want := "server_name blog.example.com;"; !strings.Contains(configText, want) {
		t.Fatalf("expected config to contain %q, got:\n%s", want, configText)
	}
	if want := "proxy_pass http://127.0.0.1:8085;"; !strings.Contains(configText, want) {
		t.Fatalf("expected config to contain %q, got:\n%s", want, configText)
	}
	if want := "grpc_pass grpc://127.0.0.1:8085;"; !strings.Contains(configText, want) {
		t.Fatalf("expected config to contain %q, got:\n%s", want, configText)
	}
	if want := "location /grpc-custom {"; !strings.Contains(configText, want) {
		t.Fatalf("expected config to contain %q, got:\n%s", want, configText)
	}
	if want := "location ^~ /.well-known/acme-challenge/ {"; !strings.Contains(configText, want) {
		t.Fatalf("expected config to contain %q, got:\n%s", want, configText)
	}
}

func TestBuildCoreConfigDisablesBackendTLSWhenNginxManagesConnection(t *testing.T) {
	registry := newProtocolRegistry()
	secrets, err := registry.GenerateSecrets("pp-fallback")
	if err != nil {
		t.Fatalf("GenerateSecrets() error = %v", err)
	}
	connection := Connection{
		ID:       1,
		Name:     "blog",
		Tag:      "blog",
		Protocol: "pp-fallback",
		Listen:   "127.0.0.1:8085",
		Enabled:  true,
		TLS:      &config.TLSConfig{Enabled: true, CertFile: "/tmp/cert.pem", KeyFile: "/tmp/key.pem"},
		Settings: map[string]any{
			"domain":            "blog.example.com",
			"psk":               secrets["psk"],
			"noise_private_key": secrets["noise_private_key"],
			"scraper_keywords":  []string{"news"},
		},
	}

	cfg, err := registry.BuildCoreConfig([]Connection{connection}, nil)
	if err != nil {
		t.Fatalf("BuildCoreConfig() error = %v", err)
	}
	if len(cfg.Inbounds) != 1 {
		t.Fatalf("expected one inbound, got %d", len(cfg.Inbounds))
	}
	if cfg.Inbounds[0].TLS != nil {
		t.Fatalf("expected backend TLS to be disabled when nginx manages the connection")
	}
}

func TestBuildCoreConfigPreservesTLSForDirectHTTPSConnection(t *testing.T) {
	registry := newProtocolRegistry()
	secrets, err := registry.GenerateSecrets("pp-fallback")
	if err != nil {
		t.Fatalf("GenerateSecrets() error = %v", err)
	}
	connection := Connection{
		ID:       1,
		Name:     "direct",
		Tag:      "direct",
		Protocol: "pp-fallback",
		Listen:   "0.0.0.0:443",
		Enabled:  true,
		TLS:      &config.TLSConfig{Enabled: true, CertFile: "/tmp/cert.pem", KeyFile: "/tmp/key.pem"},
		Settings: map[string]any{
			"domain":            "direct.example.com",
			"psk":               secrets["psk"],
			"noise_private_key": secrets["noise_private_key"],
			"scraper_keywords":  []string{"news"},
		},
	}

	cfg, err := registry.BuildCoreConfig([]Connection{connection}, nil)
	if err != nil {
		t.Fatalf("BuildCoreConfig() error = %v", err)
	}
	if len(cfg.Inbounds) != 1 {
		t.Fatalf("expected one inbound, got %d", len(cfg.Inbounds))
	}
	if cfg.Inbounds[0].TLS == nil || !cfg.Inbounds[0].TLS.Enabled {
		t.Fatalf("expected direct :443 connection to keep backend TLS enabled")
	}
}
