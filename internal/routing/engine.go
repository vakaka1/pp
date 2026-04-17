package routing

import (
	"net"

	"github.com/user/pp/internal/config"
)

type Policy string

const (
	PolicyDirect Policy = "direct"
	PolicyProxy  Policy = "proxy"
	PolicyBlock  Policy = "block"
)

// Engine is the routing engine that decides how to route a connection.
type Engine struct {
	rules         []Rule
	defaultPolicy Policy
}

type Rule struct {
	Matcher Matcher
	Policy  Policy
}

// NewEngine creates a new routing engine based on the configuration.
func NewEngine(cfg config.RoutingConfig, geoip *GeoIPDB, geosite *GeoSiteDB) (*Engine, error) {
	engine := &Engine{
		defaultPolicy: Policy(cfg.DefaultPolicy),
	}
	if engine.defaultPolicy == "" {
		engine.defaultPolicy = PolicyProxy
	}

	for _, r := range cfg.Rules {
		matcher, err := CreateMatcher(r.Type, r.Value, geoip, geosite)
		if err != nil {
			return nil, err
		}
		engine.rules = append(engine.rules, Rule{
			Matcher: matcher,
			Policy:  Policy(r.Policy),
		})
	}
	return engine, nil
}

// Route determines the policy for the given host and/or IP.
func (e *Engine) Route(host string, ip net.IP) Policy {
	for _, rule := range e.rules {
		if rule.Matcher.Match(host, ip) {
			return rule.Policy
		}
	}
	return e.defaultPolicy
}
