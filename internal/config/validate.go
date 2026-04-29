package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/vakaka1/pp/internal/crypto"
)

// Validate checks the loaded configuration for errors.
func (c *Config) Validate(isServer bool) error {
	if isServer {
		if len(c.Inbounds) == 0 {
			return fmt.Errorf("inbounds configuration is missing or empty")
		}
		return validateInboundsConfig(c.Inbounds)
	} else {
		if c.Client == nil {
			return fmt.Errorf("client configuration is missing")
		}
		return validateClientConfig(c.Client)
	}
}

func validateInboundsConfig(inbounds []InboundConfig) error {
	for i, inb := range inbounds {
		if inb.Listen == "" {
			return fmt.Errorf("inbounds[%d].listen is required", i)
		}
		if _, err := net.ResolveTCPAddr("tcp", inb.Listen); err != nil {
			return fmt.Errorf("invalid inbounds[%d].listen address: %w", i, err)
		}

		if inb.Protocol == "pp-fallback" {
			var settings FallbackSettings
			if err := json.Unmarshal(inb.Settings, &settings); err != nil {
				return fmt.Errorf("inbounds[%d]: failed to parse pp-fallback settings: %w", i, err)
			}

			if settings.Domain == "" {
				return fmt.Errorf("inbounds[%d].settings.domain is required for pp-fallback", i)
			}

			// At least one PSK must be configured: either the single psk or a non-empty psks list.
			if len(settings.PSKs) > 0 {
				for j, psk := range settings.PSKs {
					if err := validateKey(psk, fmt.Sprintf("inbounds[%d].settings.psks[%d]", i, j)); err != nil {
						return err
					}
				}
			} else {
				if err := validateKey(settings.PSK, fmt.Sprintf("inbounds[%d].settings.psk", i)); err != nil {
					return err
				}
			}

			if err := validateKey(settings.NoisePrivateKey, fmt.Sprintf("inbounds[%d].settings.noise_private_key", i)); err != nil {
				return err
			}

			fallbackType := strings.TrimSpace(settings.Type)
			if fallbackType == "" {
				fallbackType = "blog"
			}
			// Content scraping requires at least one keyword when operating as a blog or forum.
			if fallbackType != "proxy" && len(compactNonEmptyStrings(settings.ScraperKeywords)) == 0 {
				return fmt.Errorf("inbounds[%d].settings.scraper_keywords: at least one keyword is required for type %q", i, fallbackType)
			}
			if settings.PublishIntervalMinutes < 0 {
				return fmt.Errorf("inbounds[%d].settings.publish_interval_minutes must be >= 0", i)
			}
			if settings.PublishMinDelayMinutes < 0 {
				return fmt.Errorf("inbounds[%d].settings.publish_min_delay_minutes must be >= 0", i)
			}
			if settings.PublishMaxDelayMinutes < 0 {
				return fmt.Errorf("inbounds[%d].settings.publish_max_delay_minutes must be >= 0", i)
			}
			if settings.PublishBatchSize < 0 {
				return fmt.Errorf("inbounds[%d].settings.publish_batch_size must be >= 0", i)
			}
			minDelay, maxDelay := ResolveFallbackPublishWindow(&settings)
			if minDelay <= 0 || maxDelay <= 0 {
				return fmt.Errorf("inbounds[%d].settings publish window must resolve to positive values", i)
			}
			if settings.Routing != nil {
				if err := validateRoutingConfig(settings.Routing.DefaultPolicy, settings.Routing.Rules, fmt.Sprintf("inbounds[%d].settings.routing", i)); err != nil {
					return err
				}
			}
		} else {
			return fmt.Errorf("inbounds[%d]: unsupported protocol '%s'", i, inb.Protocol)
		}
	}

	return nil
}

func validateClientConfig(c *ClientConfig) error {
	if c.Socks5Listen == "" && c.HTTPProxyListen == "" && c.TransparentListen == "" {
		return fmt.Errorf("at least one of client.socks5_listen, client.http_proxy_listen or client.transparent_listen is required")
	}

	if c.Server.Address == "" {
		return fmt.Errorf("client.server.address is required")
	}

	if err := validateKey(c.Server.PSK, "client.server.psk"); err != nil {
		return err
	}

	if err := validateKey(c.Server.NoisePublicKey, "client.server.noise_public_key"); err != nil {
		return err
	}

	if c.Routing != nil {
		if err := validateRoutingConfig(c.Routing.DefaultPolicy, c.Routing.Rules, "client.routing"); err != nil {
			return err
		}
	}

	return nil
}

func validateKey(keyBase64, fieldName string) error {
	if keyBase64 == "" {
		return fmt.Errorf("%s is required", fieldName)
	}
	keyBytes, err := base64.RawURLEncoding.DecodeString(keyBase64)
	if err != nil {
		return fmt.Errorf("%s: invalid base64url encoding: %w", fieldName, err)
	}
	if len(keyBytes) != 32 {
		return fmt.Errorf("%s: must be exactly 32 bytes after decoding, got %d", fieldName, len(keyBytes))
	}
	// Attempt decode with crypto package for safety
	if _, err := crypto.DecodeKey(keyBase64); err != nil {
		return fmt.Errorf("%s: %w", fieldName, err)
	}
	return nil
}

func compactNonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func validateRoutingConfig(defaultPolicy string, rules []RoutingRule, fieldPrefix string) error {
	if defaultPolicy != "" && !isValidRoutingPolicy(defaultPolicy) {
		return fmt.Errorf("%s.default_policy must be one of: direct, proxy, block", fieldPrefix)
	}

	for i, rule := range rules {
		if rule.Type == "" {
			return fmt.Errorf("%s.rules[%d].type is required", fieldPrefix, i)
		}
		if rule.Value == "" {
			return fmt.Errorf("%s.rules[%d].value is required", fieldPrefix, i)
		}
		if !isValidRoutingPolicy(rule.Policy) {
			return fmt.Errorf("%s.rules[%d].policy must be one of: direct, proxy, block", fieldPrefix, i)
		}
		if rule.Type == "ip_cidr" {
			if _, _, err := net.ParseCIDR(rule.Value); err != nil {
				return fmt.Errorf("invalid CIDR in %s.rules[%d]: %w", fieldPrefix, i, err)
			}
		}
	}

	return nil
}

func isValidRoutingPolicy(policy string) bool {
	switch policy {
	case "direct", "proxy", "block":
		return true
	default:
		return false
	}
}
