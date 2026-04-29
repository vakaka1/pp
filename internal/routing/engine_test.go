package routing

import (
	"net"
	"testing"

	"github.com/vakaka1/pp/internal/config"
)

func TestRoutingEngine(t *testing.T) {
	cfg := config.RoutingConfig{
		DefaultPolicy: "proxy",
		Rules: []config.RoutingRule{
			{Type: "domain", Value: "example.com", Policy: "direct"},
			{Type: "domain_suffix", Value: ".local", Policy: "direct"},
			{Type: "ip_cidr", Value: "192.168.0.0/16", Policy: "direct"},
			{Type: "domain_keyword", Value: "blockme", Policy: "block"},
		},
	}

	engine, err := NewEngine(cfg, nil, nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	if engine.Route("example.com", nil) != PolicyDirect {
		t.Fatalf("expected direct for example.com")
	}

	if engine.Route("test.local", nil) != PolicyDirect {
		t.Fatalf("expected direct for test.local")
	}

	if engine.Route("", net.ParseIP("192.168.1.5")) != PolicyDirect {
		t.Fatalf("expected direct for 192.168.1.5")
	}

	if engine.Route("www.blockme.com", nil) != PolicyBlock {
		t.Fatalf("expected block for www.blockme.com")
	}

	if engine.Route("google.com", nil) != PolicyProxy {
		t.Fatalf("expected default proxy for google.com")
	}
}

func TestRoutingRules(t *testing.T) {
	// DomainRegex
	reMatcher, _ := CreateMatcher("domain_regex", "^cdn[0-9]+\\.example\\.com$", nil, nil)
	if !reMatcher.Match("cdn123.example.com", nil) {
		t.Fatalf("regex match failed")
	}
	if reMatcher.Match("cdn.example.com", nil) {
		t.Fatalf("regex match should have failed")
	}

	// Geosite mock
	geositeMatcher, _ := CreateMatcher("geosite", "ru", nil, &GeoSiteDB{})
	if !geositeMatcher.Match("yandex.ru", nil) {
		t.Fatalf("geosite match failed")
	}
	if geositeMatcher.Match("google.com", nil) {
		t.Fatalf("geosite match should have failed")
	}
}
