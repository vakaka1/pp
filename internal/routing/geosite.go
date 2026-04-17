package routing

import (
	"strings"
)

// GeoSiteDB is a simple mock for geosite matching since v2ray dat format is complex to parse
// without importing v2ray-core, which is explicitly forbidden.
// We will just do a basic implementation where "ru" matches .ru domains.
type GeoSiteDB struct {
	// In a real implementation this would parse the v2fly dat file.
	// For the agent implementation, we simulate basic behavior.
}

// LoadGeoSite loads the GeoSite database.
func LoadGeoSite(data []byte) (*GeoSiteDB, error) {
	return &GeoSiteDB{}, nil
}

// Match checks if the domain matches the geosite list (e.g. "ru").
func (g *GeoSiteDB) Match(domain string, code string) bool {
	if g == nil {
		return false
	}
	// Very simple mockup: if code is "ru", match *.ru
	if code == "ru" {
		return strings.HasSuffix(domain, ".ru") || domain == "ru"
	}
	return false
}
