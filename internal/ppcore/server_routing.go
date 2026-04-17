package ppcore

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/user/pp/internal/config"
	"github.com/user/pp/internal/routing"
	"github.com/xtaci/smux"
	"go.uber.org/zap"
)

func newInboundStreamHandler(engine *routing.Engine) func(*smux.Stream, *zap.Logger) {
	return func(stream *smux.Stream, log *zap.Logger) {
		serveTunnelStream(stream, log, engine)
	}
}

func buildServerRoutingEngine(settings config.FallbackSettings, geoIP *routing.GeoIPDB, geoSite *routing.GeoSiteDB) (*routing.Engine, error) {
	if settings.Routing == nil {
		return nil, nil
	}

	cfg := config.RoutingConfig{
		DefaultPolicy: settings.Routing.DefaultPolicy,
		Rules:         settings.Routing.Rules,
	}
	return routing.NewEngine(cfg, geoIP, geoSite)
}

func loadServerRoutingDatabases(log *zap.Logger) (*routing.GeoIPDB, *routing.GeoSiteDB) {
	return loadGeoIPDatabase(filepath.Join("data", "geoip.dat"), log), loadGeoSiteDatabase(filepath.Join("data", "geosite.dat"), log)
}

func loadGeoIPDatabase(path string, log *zap.Logger) *routing.GeoIPDB {
	data, err := os.ReadFile(path)
	if err != nil {
		if log != nil {
			log.Warn("server GeoIP database is unavailable; geoip rules will not match", zap.String("path", path), zap.Error(err))
		}
		return nil
	}

	db, err := routing.LoadGeoIP(data)
	if err != nil {
		if log != nil {
			log.Warn("failed to load server GeoIP database; geoip rules will not match", zap.String("path", path), zap.Error(err))
		}
		return nil
	}
	return db
}

func loadGeoSiteDatabase(path string, log *zap.Logger) *routing.GeoSiteDB {
	data, err := os.ReadFile(path)
	if err != nil {
		if log != nil {
			log.Warn("server GeoSite database is unavailable; geosite rules will not match", zap.String("path", path), zap.Error(err))
		}
		return nil
	}

	db, err := routing.LoadGeoSite(data)
	if err != nil {
		if log != nil {
			log.Warn("failed to load server GeoSite database; geosite rules will not match", zap.String("path", path), zap.Error(err))
		}
		return nil
	}
	return db
}

func decodeFallbackSettings(inb config.InboundConfig) (config.FallbackSettings, error) {
	var settings config.FallbackSettings
	if err := json.Unmarshal(inb.Settings, &settings); err != nil {
		return config.FallbackSettings{}, err
	}
	return settings, nil
}
