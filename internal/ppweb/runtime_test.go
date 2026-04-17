package ppweb

import (
	"strings"
	"testing"

	"github.com/user/pp/internal/config"
)

func TestShouldManageNginx(t *testing.T) {
	server := &Server{}

	loopback := &Connection{
		Enabled: true,
		Listen:  "127.0.0.1:8081",
		TLS:     &config.TLSConfig{Enabled: true},
	}
	if !server.shouldManageNginx(loopback) {
		t.Fatalf("expected loopback TLS connection to be published through nginx")
	}

	direct := &Connection{
		Enabled: true,
		Listen:  "0.0.0.0:443",
		TLS:     &config.TLSConfig{Enabled: true},
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
	if want := "proxy_pass https://127.0.0.1:8085;"; !strings.Contains(configText, want) {
		t.Fatalf("expected config to contain %q, got:\n%s", want, configText)
	}
	if want := "location /grpc-custom {"; !strings.Contains(configText, want) {
		t.Fatalf("expected config to contain %q, got:\n%s", want, configText)
	}
}
