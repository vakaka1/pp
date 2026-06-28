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

	if engine.Route("www.example.com", nil) != PolicyDirect {
		t.Fatalf("expected direct for www.example.com")
	}

	if engine.Route("EXAMPLE.COM.", nil) != PolicyDirect {
		t.Fatalf("expected direct for normalized example.com")
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

func TestRoutingDomainRuleMatchesApexAndSubdomains(t *testing.T) {
	engine, err := NewEngine(config.RoutingConfig{
		DefaultPolicy: "block",
		Rules: []config.RoutingRule{
			{Type: "domain", Value: "2ip.ru", Policy: "direct"},
		},
	}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	for _, host := range []string{"2ip.ru", "www.2ip.ru", "WWW.2IP.RU."} {
		if engine.Route(host, nil) != PolicyDirect {
			t.Fatalf("expected direct for %s", host)
		}
	}
}

func TestGeoIPRuleWithoutDatabaseDoesNotMatch(t *testing.T) {
	matcher, err := CreateMatcher("geoip", "ru", nil, nil)
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}
	if matcher.Match("", net.ParseIP("8.8.8.8")) {
		t.Fatalf("geoip matcher without database must not match")
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

	legacyReMatcher, err := CreateMatcher("regexp", "^api\\.example\\.com$", nil, nil)
	if err != nil {
		t.Fatalf("legacy regexp matcher failed: %v", err)
	}
	if !legacyReMatcher.Match("api.example.com", nil) {
		t.Fatalf("legacy regexp match failed")
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
