package routing

import (
	"net"
	"strings"

	"github.com/oschwald/maxminddb-golang"
)

// GeoIPDB is a wrapper around MaxMind DB for routing.
type GeoIPDB struct {
	db *maxminddb.Reader
}

// LoadGeoIP loads the GeoIP database from bytes.
func LoadGeoIP(data []byte) (*GeoIPDB, error) {
	db, err := maxminddb.FromBytes(data)
	if err != nil {
		return nil, err
	}
	return &GeoIPDB{db: db}, nil
}

// Match checks if the IP belongs to the specified country code.
// For v2ray geoip format, it uses an internal lookup but MaxMind DB uses "country.iso_code".
// For this simple implementation we assume MaxMind DB format (e.g. Country DB).
func (g *GeoIPDB) Match(ip net.IP, code string) bool {
	if g == nil || g.db == nil || ip == nil {
		return false
	}
	var record struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
		RegisteredCountry struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"registered_country"`
	}
	err := g.db.Lookup(ip, &record)
	if err != nil {
		return false
	}
	code = strings.ToUpper(code)
	return record.Country.ISOCode == code || record.RegisteredCountry.ISOCode == code
}

func (g *GeoIPDB) Close() error {
	if g.db != nil {
		return g.db.Close()
	}
	return nil
}
