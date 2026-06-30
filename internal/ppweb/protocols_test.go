package ppweb

import (
	"encoding/base64"
	"testing"
)

func TestFallbackProtocolDescriptorDoesNotExposeForumOption(t *testing.T) {
	descriptor := (fallbackProtocol{}).Descriptor()

	for _, section := range descriptor.Sections {
		for _, field := range section.Fields {
			if field.Path != "type" {
				continue
			}

			for _, option := range field.Options {
				if option.Value == "forum" {
					t.Fatalf("unexpected forum option in fallback protocol descriptor")
				}
			}
			return
		}
	}

	t.Fatalf("type field not found in fallback protocol descriptor")
}

func TestFallbackClientConfigIncludesRoutingSnapshot(t *testing.T) {
	key := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	result, err := (fallbackProtocol{}).BuildClientConfigForClient(Connection{
		Settings: map[string]any{
			"type":              "blog",
			"domain":            "example.com",
			"grpc_path":         "/login",
			"noise_private_key": key,
			"routing": map[string]any{
				"default_policy": "proxy",
				"rules": []any{
					map[string]any{"type": "domain", "value": "example.org", "policy": "direct"},
				},
			},
		},
	}, "client", key)
	if err != nil {
		t.Fatalf("BuildClientConfigForClient failed: %v", err)
	}

	clientConfig := result.(ClientConfigResult).Config.Client
	if clientConfig.Routing == nil {
		t.Fatalf("expected client routing snapshot")
	}
	if clientConfig.Routing.DefaultPolicy != "proxy" {
		t.Fatalf("unexpected default policy: %s", clientConfig.Routing.DefaultPolicy)
	}
	if len(clientConfig.Routing.Rules) != 1 || clientConfig.Routing.Rules[0].Policy != "direct" {
		t.Fatalf("unexpected routing rules: %#v", clientConfig.Routing.Rules)
	}
}
