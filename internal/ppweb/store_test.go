package ppweb

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/user/pp/internal/config"
)

func TestSaveConnectionPreservesTLSWhenUpdateOmitsIt(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "pp-web.sqlite"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	created, err := store.SaveConnection(context.Background(), 0, ConnectionInput{
		Name:     "blog",
		Tag:      "blog",
		Protocol: "pp-fallback",
		Listen:   ":8081",
		TLS:      &config.TLSConfig{Enabled: true, CertFile: "/tmp/cert.pem", KeyFile: "/tmp/key.pem"},
		Enabled:  true,
		Settings: map[string]any{
			"type":              "blog",
			"domain":            "blog.example.com",
			"noise_private_key": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"psk":               "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"scraper_keywords":  []string{"go"},
		},
	})
	if err != nil {
		t.Fatalf("SaveConnection(create) error = %v", err)
	}

	updated, err := store.SaveConnection(context.Background(), created.ID, ConnectionInput{
		Name:     created.Name,
		Tag:      created.Tag,
		Protocol: created.Protocol,
		Listen:   created.Listen,
		Enabled:  created.Enabled,
		Settings: created.Settings,
	})
	if err != nil {
		t.Fatalf("SaveConnection(update) error = %v", err)
	}

	if updated.TLS == nil || !updated.TLS.Enabled {
		t.Fatalf("expected TLS to be preserved on update without tls payload, got %#v", updated.TLS)
	}
	if updated.TLS.CertFile != "/tmp/cert.pem" || updated.TLS.KeyFile != "/tmp/key.pem" {
		t.Fatalf("unexpected preserved TLS config: %#v", updated.TLS)
	}
}
