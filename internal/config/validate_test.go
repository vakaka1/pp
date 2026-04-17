package config

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestConfigValidationServer(t *testing.T) {
	key := base64.RawURLEncoding.EncodeToString(make([]byte, 32))

	settings := FallbackSettings{
		Type:            "blog",
		Domain:          "example.com",
		GRPCPath:        "/pp.v1.TunnelService/Connect",
		NoisePrivateKey: key,
		PSK:             key,
		ScraperKeywords: []string{"golang"},
	}
	rawSettings, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	cfg := &Config{
		Inbounds: []InboundConfig{
			{
				Tag:      "main",
				Protocol: "pp-fallback",
				Listen:   "127.0.0.1:8443",
				Settings: rawSettings,
			},
		},
	}

	if err := cfg.Validate(true); err != nil {
		t.Fatalf("expected valid server config, got: %v", err)
	}

	settings.PSK = base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	rawSettings, err = json.Marshal(settings)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	cfg.Inbounds[0].Settings = rawSettings

	if err := cfg.Validate(true); err == nil {
		t.Fatalf("expected error for invalid PSK length")
	}

	settings.PSK = key
	settings.ScraperKeywords = nil
	rawSettings, err = json.Marshal(settings)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	cfg.Inbounds[0].Settings = rawSettings

	if err := cfg.Validate(true); err == nil {
		t.Fatalf("expected error for missing scraper keywords")
	}
}

func TestConfigValidationClient(t *testing.T) {
	key := base64.RawURLEncoding.EncodeToString(make([]byte, 32))

	cfg := &Config{
		Client: &ClientConfig{
			Socks5Listen: "127.0.0.1:1080",
		},
	}

	cfg.Client.Server.Address = "example.com:443"
	cfg.Client.Server.PSK = key
	cfg.Client.Server.NoisePublicKey = key

	if err := cfg.Validate(false); err != nil {
		t.Fatalf("expected valid client config, got: %v", err)
	}

	cfg.Client.Routing = &RoutingConfig{
		Rules: []RoutingRule{
			{Type: "ip_cidr", Value: "192.168.1.1/abc", Policy: "direct"},
		},
	}

	if err := cfg.Validate(false); err == nil {
		t.Fatalf("expected error for invalid CIDR")
	}

	cfg = &Config{
		Client: &ClientConfig{
			TransparentListen: "127.0.0.1:1090",
		},
	}
	cfg.Client.Server.Address = "example.com:443"
	cfg.Client.Server.PSK = key
	cfg.Client.Server.NoisePublicKey = key

	if err := cfg.Validate(false); err != nil {
		t.Fatalf("expected transparent-only client config to be valid, got: %v", err)
	}
}
