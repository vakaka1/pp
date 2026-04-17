package routing

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// Matcher represents a routing rule matcher.
type Matcher interface {
	Match(host string, ip net.IP) bool
}

// DomainMatcher matches exact domains.
type DomainMatcher struct {
	Domain string
}

func (m *DomainMatcher) Match(host string, ip net.IP) bool { return m.Domain == host }

// DomainSuffixMatcher matches domains ending with a suffix.
type DomainSuffixMatcher struct {
	Suffix string
}

func (m *DomainSuffixMatcher) Match(host string, ip net.IP) bool {
	return strings.HasSuffix(host, m.Suffix) || host == strings.TrimPrefix(m.Suffix, ".")
}

// DomainKeywordMatcher matches domains containing a keyword.
type DomainKeywordMatcher struct {
	Keyword string
}

func (m *DomainKeywordMatcher) Match(host string, ip net.IP) bool {
	return strings.Contains(host, m.Keyword)
}

// DomainRegexMatcher matches domains against a regex.
type DomainRegexMatcher struct {
	Regex *regexp.Regexp
}

func (m *DomainRegexMatcher) Match(host string, ip net.IP) bool { return m.Regex.MatchString(host) }

// IPCIDRMatcher matches IPs against a CIDR.
type IPCIDRMatcher struct {
	Net *net.IPNet
}

func (m *IPCIDRMatcher) Match(host string, ip net.IP) bool {
	if ip == nil {
		return false
	}
	return m.Net.Contains(ip)
}

// GeoIPMatcher matches IPs against a GeoIP code.
type GeoIPMatcher struct {
	Code string
	DB   *GeoIPDB
}

func (m *GeoIPMatcher) Match(host string, ip net.IP) bool {
	if ip == nil {
		return false
	}
	return m.DB.Match(ip, m.Code)
}

// GeoSiteMatcher matches domains against a GeoSite list.
type GeoSiteMatcher struct {
	Code string
	DB   *GeoSiteDB
}

func (m *GeoSiteMatcher) Match(host string, ip net.IP) bool {
	if host == "" {
		return false
	}
	if m.DB == nil {
		return false
	}
	return m.DB.Match(host, m.Code)
}

func CreateMatcher(ruleType, value string, geoip *GeoIPDB, geosite *GeoSiteDB) (Matcher, error) {
	switch ruleType {
	case "domain":
		return &DomainMatcher{Domain: value}, nil
	case "domain_suffix":
		return &DomainSuffixMatcher{Suffix: value}, nil
	case "domain_keyword":
		return &DomainKeywordMatcher{Keyword: value}, nil
	case "domain_regex":
		re, err := regexp.Compile(value)
		if err != nil {
			return nil, err
		}
		return &DomainRegexMatcher{Regex: re}, nil
	case "ip_cidr":
		_, ipNet, err := net.ParseCIDR(value)
		if err != nil {
			return nil, err
		}
		return &IPCIDRMatcher{Net: ipNet}, nil
	case "geoip":
		return &GeoIPMatcher{Code: strings.ToLower(value), DB: geoip}, nil
	case "geosite":
		return &GeoSiteMatcher{Code: strings.ToLower(value), DB: geosite}, nil
	default:
		return nil, fmt.Errorf("unsupported rule type: %s", ruleType)
	}
}
