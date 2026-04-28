package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// LogConfig represents the logging configuration.
type LogConfig struct {
	Level              string `json:"level"`
	Output             string `json:"output"`
	File               string `json:"file"`
	LogTargetAddresses bool   `json:"log_target_addresses"` // Client only
}

// ConfigMeta contains human-readable metadata about this config file.
// It is written only into client configs; server-side inbound configs omit it.
// The pp client binary ignores unknown top-level fields, so this is safe.
type ConfigMeta struct {
	ClientName  string `json:"client_name,omitempty"`
	Protocol    string `json:"protocol,omitempty"`
	GeneratedAt string `json:"generated_at,omitempty"`
}

// Config represents the root configuration.
type Config struct {
	Meta     *ConfigMeta     `json:"meta,omitempty"`
	Log      LogConfig       `json:"log"`
	Inbounds []InboundConfig `json:"inbounds,omitempty"`
	Client   *ClientConfig   `json:"client,omitempty"`
}

// InboundConfig represents an incoming connection handler configuration.
type InboundConfig struct {
	Tag      string          `json:"tag"`
	Protocol string          `json:"protocol"`
	Listen   string          `json:"listen"`
	TLS      *TLSConfig      `json:"tls,omitempty"`
	Settings json.RawMessage `json:"settings"`
}

type TLSConfig struct {
	Enabled  bool   `json:"enabled"`
	CertFile string `json:"cert_file,omitempty"`
	KeyFile  string `json:"key_file,omitempty"`
}

// FallbackSettings represents the settings for the "pp-fallback" protocol.
type FallbackSettings struct {
	Type            string `json:"type"`
	Domain          string `json:"domain"`
	GRPCPath        string `json:"grpc_path"`
	NoisePrivateKey string `json:"noise_private_key"`
	PSK             string `json:"psk"`
	// PSKs is a list of per-client pre-shared keys. When non-empty, the server
	// accepts any JWT signed by one of these keys (ignoring the single PSK field).
	PSKs                   []string         `json:"psks,omitempty"`
	ProxyAddress           string           `json:"proxy_address,omitempty"`
	DBPath                 string           `json:"db_path,omitempty"`
	RSSSources             []string         `json:"rss_sources,omitempty"`
	ScraperKeywords        []string         `json:"scraper_keywords,omitempty"`
	PublishIntervalMinutes int              `json:"publish_interval_minutes,omitempty"` // legacy fixed interval input
	PublishMinDelayMinutes int              `json:"publish_min_delay_minutes,omitempty"`
	PublishMaxDelayMinutes int              `json:"publish_max_delay_minutes,omitempty"`
	PublishBatchSize       int              `json:"publish_batch_size,omitempty"`
	InviteCode             string           `json:"invite_code,omitempty"`
	Limits                 LimitsConfig     `json:"limits"`
	AntiReplay             AntiReplayConfig `json:"anti_replay"`
	// Routing defines the server-side routing policy for this connection.
	// When set, the inbound handler enforces these rules for all clients.
	// Clients themselves send all traffic to the server without local rules.
	Routing *ServerRoutingConfig `json:"routing,omitempty"`
}

// ServerRoutingConfig defines the routing policy enforced server-side for this
// connection's inbound handler. Clients send all traffic through the tunnel;
// the server applies these rules to decide what to allow, block, or direct.
type ServerRoutingConfig struct {
	DefaultPolicy string        `json:"default_policy"` // "proxy" (allow) or "block"
	Rules         []RoutingRule `json:"rules,omitempty"`
}

type LimitsConfig struct {
	MaxConnections     int `json:"max_connections"`
	MaxStreamsPerConn  int `json:"max_streams_per_conn"`
	IdleTimeoutSeconds int `json:"idle_timeout_seconds"`
	MaxBandwidthMbps   int `json:"max_bandwidth_mbps"`
}

type AntiReplayConfig struct {
	BloomCapacity   uint    `json:"bloom_capacity"`
	BloomErrorRate  float64 `json:"bloom_error_rate"`
	RotationMinutes int     `json:"rotation_minutes"`
}

// ClientConfig represents the client configuration.
type ClientConfig struct {
	Socks5Listen      string `json:"socks5_listen"`
	HTTPProxyListen   string `json:"http_proxy_listen"`
	TransparentListen string `json:"transparent_listen,omitempty"`

	Server struct {
		Address        string `json:"address"`
		Domain         string `json:"domain"`
		NoisePublicKey string `json:"noise_public_key"`
		PSK            string `json:"psk"`
		TLSFingerprint string `json:"tls_fingerprint"`
		GRPCPath       string `json:"grpc_path"`
		GRPCUserAgent  string `json:"grpc_user_agent"`
	} `json:"server"`

	// Routing is used only in server-side inbound config, not in client configs.
	// Client configs omit this field entirely — routing is centralized on the server.
	Routing *RoutingConfig `json:"routing,omitempty"`

	Transport struct {
		JitterMaxMs              int  `json:"jitter_max_ms"`
		ShaperEnabled            bool `json:"shaper_enabled"`
		KeepaliveIntervalSeconds int  `json:"keepalive_interval_seconds"`
		ReconnectStreamMin       int  `json:"reconnect_stream_min"`
		ReconnectStreamMax       int  `json:"reconnect_stream_max"`
		ReconnectDurationMinH    int  `json:"reconnect_duration_min_h"`
		ReconnectDurationMaxH    int  `json:"reconnect_duration_max_h"`
	} `json:"transport"`

	ConnectionPool struct {
		Size int `json:"size"`
	} `json:"connection_pool"`
}

type RoutingConfig struct {
	DefaultPolicy string `json:"default_policy"`
	DNS           struct {
		Strategy     string   `json:"strategy"`
		LocalServers []string `json:"local_servers"`
		DohURL       string   `json:"doh_url"`
	} `json:"dns"`
	Rules []RoutingRule `json:"rules"`
}

type RoutingRule struct {
	Comment string `json:"comment,omitempty"`
	Type    string `json:"type"`
	Value   string `json:"value"`
	Policy  string `json:"policy"`
}

// LoadConfig reads the configuration from a JSON file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	return &cfg, nil
}
