package ppweb

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	ppconfig "github.com/vakaka1/pp/internal/config"
	ppcrypto "github.com/vakaka1/pp/internal/crypto"
)

type protocolDriver interface {
	Descriptor() ProtocolDescriptor
	Normalize(ConnectionInput) (ConnectionInput, error)
	Inbound(Connection) (ppconfig.InboundConfig, error)
	GenerateSecrets() (map[string]string, error)
	BuildClientConfig(Connection) (any, error)
	// BuildClientConfigForClient builds a client config with the client's name and unique PSK.
	// Returns a ClientConfigResult with both a compact URI and full JSON config.
	BuildClientConfigForClient(Connection, string, string) (any, error)
}

type protocolRegistry struct {
	drivers map[string]protocolDriver
}

func newProtocolRegistry() *protocolRegistry {
	registry := &protocolRegistry{
		drivers: map[string]protocolDriver{},
	}
	registry.drivers["pp-fallback"] = fallbackProtocol{}
	return registry
}

func (r *protocolRegistry) Descriptors() []ProtocolDescriptor {
	descriptors := make([]ProtocolDescriptor, 0, len(r.drivers))
	for _, id := range []string{"pp-fallback"} {
		if driver, ok := r.drivers[id]; ok {
			descriptors = append(descriptors, driver.Descriptor())
		}
	}
	return descriptors
}

func (r *protocolRegistry) NormalizeConnection(input ConnectionInput) (ConnectionInput, error) {
	driver, ok := r.drivers[input.Protocol]
	if !ok {
		return ConnectionInput{}, fmt.Errorf("unsupported protocol %q", input.Protocol)
	}
	return driver.Normalize(input)
}

// BuildCoreConfig builds the pp-core JSON config from all enabled connections.
// clientsByConn maps connection ID to per-client credentials accepted by that
// connection's inbound. When non-empty, the generated settings track clients
// individually so pp-fallback can authenticate and report their online status.
func (r *protocolRegistry) BuildCoreConfig(connections []Connection, clientsByConn map[int64][]Client, statusPath string) (*ppconfig.Config, error) {
	cfg := &ppconfig.Config{
		Log: ppconfig.LogConfig{
			Level:  "info",
			Output: "stdout",
		},
		Inbounds: []ppconfig.InboundConfig{},
	}

	for _, connection := range connections {
		if !connection.Enabled {
			continue
		}

		driver, ok := r.drivers[connection.Protocol]
		if !ok {
			return nil, fmt.Errorf("unsupported protocol %q", connection.Protocol)
		}

		inbound, err := driver.Inbound(connection)
		if err != nil {
			return nil, fmt.Errorf("connection %q: %w", connection.Name, err)
		}
		if shouldManageNginxConnection(&connection) {
			// In managed mode nginx terminates TLS on :443 and forwards plain h2c/http
			// to pp-core on the loopback/backend port.
			inbound.TLS = nil
		} else {
			inbound.TLS = connection.TLS
		}

		// Inject per-client identities when available.
		if clients := clientsByConn[connection.ID]; len(clients) > 0 {
			var s ppconfig.FallbackSettings
			if err := json.Unmarshal(inbound.Settings, &s); err == nil {
				s.Clients = make([]ppconfig.FallbackClient, 0, len(clients))
				for _, client := range clients {
					if client.PSK == "" {
						continue
					}
					s.Clients = append(s.Clients, ppconfig.FallbackClient{
						ID:   client.ID,
						Name: client.Name,
						PSK:  client.PSK,
					})
				}
				s.PSKs = nil
				s.PSK = "" // clear single PSK – server now uses the list
				s.StatusPath = statusPath
				if raw, err := json.Marshal(s); err == nil {
					inbound.Settings = raw
				}
			}
		}

		cfg.Inbounds = append(cfg.Inbounds, inbound)
	}

	return cfg, nil
}

func (r *protocolRegistry) GenerateSecrets(protocolID string) (map[string]string, error) {
	driver, ok := r.drivers[protocolID]
	if !ok {
		return nil, fmt.Errorf("unsupported protocol %q", protocolID)
	}
	return driver.GenerateSecrets()
}

func (r *protocolRegistry) BuildClientConfig(connection Connection) (any, error) {
	driver, ok := r.drivers[connection.Protocol]
	if !ok {
		return nil, fmt.Errorf("unsupported protocol %q", connection.Protocol)
	}
	return driver.BuildClientConfig(connection)
}

// BuildClientConfigForClient builds a client config using the given client's name and unique PSK.
func (r *protocolRegistry) BuildClientConfigForClient(connection Connection, clientName, clientPSK string) (any, error) {
	driver, ok := r.drivers[connection.Protocol]
	if !ok {
		return nil, fmt.Errorf("unsupported protocol %q", connection.Protocol)
	}
	return driver.BuildClientConfigForClient(connection, clientName, clientPSK)
}

type fallbackProtocol struct{}

func (fallbackProtocol) Descriptor() ProtocolDescriptor {
	return ProtocolDescriptor{
		ID:          "pp-fallback",
		Name:        "PP Fallback",
		Summary:     "Tunnel gateway with fallback facade, keyword-driven article publishing and stealth HTTP/2 + gRPC transport.",
		StatusLabel: "Installed",
		Accent:      "#fd8956",
		Installed:   true,
		Sections: []ProtocolSection{
			{
				ID:          "mode",
				Title:       "Конфигурация",
				Description: "Определяет режим работы и публичные параметры.",
				Fields: []ProtocolField{
					selectField("type", "Тип", true, "blog", []ProtocolOption{
						{Value: "blog", Label: "Блог"},
					}),
					textField("domain", "Домен", true, "example.com", ""),
					tagsField("scraper_keywords", "Теги", false, []string{"go", "linux"}, ""),
				},
			},
		},
	}
}

func (fallbackProtocol) Normalize(input ConnectionInput) (ConnectionInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Tag = strings.TrimSpace(input.Tag)
	input.Listen = strings.TrimSpace(input.Listen)
	input.Protocol = "pp-fallback"

	if input.Name == "" {
		return ConnectionInput{}, fmt.Errorf("connection name is required")
	}
	if input.Tag == "" {
		return ConnectionInput{}, fmt.Errorf("connection tag is required")
	}
	if input.Listen == "" {
		return ConnectionInput{}, fmt.Errorf("listen address is required")
	}

	var settings ppconfig.FallbackSettings
	if err := decodeSettings(input.Settings, &settings); err != nil {
		return ConnectionInput{}, fmt.Errorf("invalid pp-fallback settings: %w", err)
	}

	settings.Type = strings.TrimSpace(settings.Type)
	settings.Domain = strings.TrimSpace(settings.Domain)
	settings.GRPCPath = strings.TrimSpace(settings.GRPCPath)
	settings.ProxyAddress = strings.TrimSpace(settings.ProxyAddress)
	settings.NoisePrivateKey = strings.TrimSpace(settings.NoisePrivateKey)
	settings.PSK = strings.TrimSpace(settings.PSK)
	settings.DBPath = strings.TrimSpace(settings.DBPath)
	settings.InviteCode = strings.TrimSpace(settings.InviteCode)

	// If project root is provided, set a default absolute DB path in /var/lib/pp or relative
	if settings.DBPath == "" || settings.DBPath == "auto" {
		fileName := "fallback-" + input.Tag + ".json"
		// We use ResolveFallbackDBPath logic implicitly or explicitly
		// But here we want to make it explicit in the config for the Core
		settings.DBPath = filepath.Join("/var/lib/pp", fileName)
	}

	settings.ScraperKeywords = trimStringSlice(settings.ScraperKeywords)
	settings.RSSSources = nil

	settings.PublishMinDelayMinutes, settings.PublishMaxDelayMinutes = ppconfig.ResolveFallbackPublishWindow(&settings)
	settings.PublishIntervalMinutes = 0
	if settings.PublishBatchSize == 0 {
		settings.PublishBatchSize = 3
	}

	if settings.Type == "" {
		settings.Type = "blog"
	}
	if settings.GRPCPath == "" {
		settings.GRPCPath = "/pp.v1.TunnelService/Connect"
	}
	if !strings.HasPrefix(settings.GRPCPath, "/") {
		return ConnectionInput{}, fmt.Errorf("grpc_path must start with '/'")
	}
	if settings.Domain == "" {
		return ConnectionInput{}, fmt.Errorf("domain is required")
	}
	if settings.Type == "proxy" && settings.ProxyAddress == "" {
		return ConnectionInput{}, fmt.Errorf("proxy_address is required when fallback type is proxy")
	}

	settings.Limits = withFallbackLimits(settings.Limits)
	settings.AntiReplay = withFallbackAntiReplay(settings.AntiReplay)

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return ConnectionInput{}, fmt.Errorf("failed to encode pp-fallback settings: %w", err)
	}

	cfg := &ppconfig.Config{
		Inbounds: []ppconfig.InboundConfig{
			{
				Tag:      input.Tag,
				Protocol: input.Protocol,
				Listen:   input.Listen,
				Settings: settingsJSON,
			},
		},
	}

	if err := cfg.Validate(true); err != nil {
		return ConnectionInput{}, err
	}

	normalized := ConnectionInput{
		Name:     input.Name,
		Tag:      input.Tag,
		Protocol: input.Protocol,
		Listen:   input.Listen,
		Enabled:  input.Enabled,
		Settings: map[string]any{},
	}
	if err := json.Unmarshal(settingsJSON, &normalized.Settings); err != nil {
		return ConnectionInput{}, fmt.Errorf("failed to normalize settings map: %w", err)
	}

	return normalized, nil
}

func (fallbackProtocol) Inbound(connection Connection) (ppconfig.InboundConfig, error) {
	normalized, err := (fallbackProtocol{}).Normalize(ConnectionInput{
		Name:     connection.Name,
		Tag:      connection.Tag,
		Protocol: connection.Protocol,
		Listen:   connection.Listen,
		Enabled:  connection.Enabled,
		Settings: connection.Settings,
	})
	if err != nil {
		return ppconfig.InboundConfig{}, err
	}

	settingsJSON, err := json.Marshal(normalized.Settings)
	if err != nil {
		return ppconfig.InboundConfig{}, fmt.Errorf("failed to encode settings: %w", err)
	}

	return ppconfig.InboundConfig{
		Tag:      normalized.Tag,
		Protocol: normalized.Protocol,
		Listen:   normalized.Listen,
		Settings: settingsJSON,
	}, nil
}

func (fallbackProtocol) GenerateSecrets() (map[string]string, error) {
	priv, pub, err := ppcrypto.GenerateX25519KeyPair()
	if err != nil {
		return nil, err
	}
	psk, err := ppcrypto.GeneratePSK()
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"noise_private_key": priv,
		"noise_public_key":  pub,
		"psk":               psk,
	}, nil
}

// ClientConfigResult is returned by BuildClientConfigForClient.
// It contains both the compact URI (for sharing) and the full JSON config
// that the pp client binary reads.
type ClientConfigResult struct {
	URI    string          `json:"uri"`
	Config ppconfig.Config `json:"config"`
}

func (fallbackProtocol) BuildClientConfig(connection Connection) (any, error) {
	var settings ppconfig.FallbackSettings
	if err := decodeSettings(connection.Settings, &settings); err != nil {
		return nil, err
	}
	// Use the connection-level PSK (legacy / no clients).
	return buildFallbackClientCfg(settings, "connection", settings.PSK)
}

func (fallbackProtocol) BuildClientConfigForClient(connection Connection, clientName, clientPSK string) (any, error) {
	var settings ppconfig.FallbackSettings
	if err := decodeSettings(connection.Settings, &settings); err != nil {
		return nil, err
	}
	cfg, err := buildFallbackClientCfg(settings, clientName, clientPSK)
	if err != nil {
		return nil, err
	}
	uri, err := buildFallbackClientURI(settings, clientName, clientPSK)
	if err != nil {
		return nil, err
	}
	result := ClientConfigResult{
		URI:    uri,
		Config: cfg,
	}
	return result, nil
}

func buildFallbackClientCfg(settings ppconfig.FallbackSettings, clientName, psk string) (ppconfig.Config, error) {
	pubKey, err := ppcrypto.DerivePublicKey(settings.NoisePrivateKey)
	if err != nil {
		return ppconfig.Config{}, fmt.Errorf("failed to derive public key: %w", err)
	}

	// Client config has NO routing rules: the client tunnels all traffic to the
	// server, and the server applies centralized routing (allow/block) based on
	// the connection's Routing settings. This means routing changes on the server
	// take effect immediately for all clients without any config update.
	clientCfg := ppconfig.Config{
		Meta: &ppconfig.ConfigMeta{
			ClientName:  clientName,
			Protocol:    "pp-fallback",
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		},
		Log: ppconfig.LogConfig{
			Level:  "info",
			Output: "stdout",
		},
		Client: &ppconfig.ClientConfig{},
	}

	clientCfg.Client.Socks5Listen = "127.0.0.1:1080"
	clientCfg.Client.HTTPProxyListen = "127.0.0.1:8080"
	clientCfg.Client.Server.Address = settings.Domain + ":443"
	clientCfg.Client.Server.Domain = settings.Domain
	clientCfg.Client.Server.NoisePublicKey = pubKey
	clientCfg.Client.Server.PSK = psk
	clientCfg.Client.Server.TLSFingerprint = "chrome"
	clientCfg.Client.Server.GRPCPath = settings.GRPCPath
	clientCfg.Client.Server.GRPCUserAgent = "grpc-go/1.62.1"
	clientCfg.Client.Transport.JitterMaxMs = 30
	clientCfg.Client.Transport.ShaperEnabled = true
	clientCfg.Client.Transport.KeepaliveIntervalSeconds = 25
	clientCfg.Client.Transport.ReconnectStreamMin = 800
	clientCfg.Client.Transport.ReconnectStreamMax = 1200
	clientCfg.Client.Transport.ReconnectDurationMinH = 3
	clientCfg.Client.Transport.ReconnectDurationMaxH = 5
	clientCfg.Client.ConnectionPool.Size = 1

	return clientCfg, nil
}

// buildFallbackClientURI generates a compact shareable URI for a pp-fallback client.
// Format: ppf://ClientName@domain:443?pub=NOISE_PUB&psk=PSK&path=GRPC_PATH&fp=chrome&proto=pp-fallback
func buildFallbackClientURI(settings ppconfig.FallbackSettings, clientName, psk string) (string, error) {
	pubKey, err := ppcrypto.DerivePublicKey(settings.NoisePrivateKey)
	if err != nil {
		return "", fmt.Errorf("failed to derive public key: %w", err)
	}

	q := url.Values{}
	q.Set("pub", pubKey)
	q.Set("psk", psk)
	q.Set("path", settings.GRPCPath)
	q.Set("fp", "chrome")
	q.Set("proto", "pp-fallback")

	u := &url.URL{
		Scheme:   "ppf",
		User:     url.User(clientName),
		Host:     settings.Domain + ":443",
		RawQuery: q.Encode(),
	}
	return u.String(), nil
}

func decodeSettings(input map[string]any, target any) error {
	if input == nil {
		input = map[string]any{}
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func trimStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func withFallbackLimits(limits ppconfig.LimitsConfig) ppconfig.LimitsConfig {
	if limits.MaxConnections == 0 {
		limits.MaxConnections = 1000
	}
	if limits.MaxStreamsPerConn == 0 {
		limits.MaxStreamsPerConn = 64
	}
	if limits.IdleTimeoutSeconds == 0 {
		limits.IdleTimeoutSeconds = 300
	}
	if limits.MaxBandwidthMbps == 0 {
		limits.MaxBandwidthMbps = 100
	}
	return limits
}

func withFallbackAntiReplay(antiReplay ppconfig.AntiReplayConfig) ppconfig.AntiReplayConfig {
	if antiReplay.BloomCapacity == 0 {
		antiReplay.BloomCapacity = 100000
	}
	if antiReplay.BloomErrorRate == 0 {
		antiReplay.BloomErrorRate = 0.001
	}
	if antiReplay.RotationMinutes == 0 {
		antiReplay.RotationMinutes = 8
	}
	return antiReplay
}

func textField(path, label string, required bool, placeholder, help string) ProtocolField {
	return ProtocolField{
		Path:        path,
		Label:       label,
		Kind:        "text",
		Required:    required,
		Placeholder: placeholder,
		Help:        help,
	}
}

func passwordField(path, label string, required bool, placeholder, help string) ProtocolField {
	return ProtocolField{
		Path:        path,
		Label:       label,
		Kind:        "password",
		Required:    required,
		Sensitive:   true,
		Placeholder: placeholder,
		Help:        help,
	}
}

func selectField(path, label string, required bool, defaultValue string, options []ProtocolOption) ProtocolField {
	return ProtocolField{
		Path:     path,
		Label:    label,
		Kind:     "select",
		Required: required,
		Default:  defaultValue,
		Options:  options,
	}
}

func tagsField(path, label string, required bool, defaultValue []string, help string) ProtocolField {
	return ProtocolField{
		Path:     path,
		Label:    label,
		Kind:     "tags",
		Required: required,
		Default:  defaultValue,
		Help:     help,
	}
}

func numberField(path, label string, required bool, defaultValue, min, max, step float64, help string) ProtocolField {
	return ProtocolField{
		Path:     path,
		Label:    label,
		Kind:     "number",
		Required: required,
		Default:  defaultValue,
		Min:      floatPtr(min),
		Max:      floatPtr(max),
		Step:     floatPtr(step),
		Help:     help,
	}
}

func floatPtr(value float64) *float64 {
	return &value
}
