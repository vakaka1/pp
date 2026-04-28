package ppweb

import "testing"

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
