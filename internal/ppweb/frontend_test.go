package ppweb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNestedFrontendAssetPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "root asset", in: "assets/index.js", want: "assets/index.js"},
		{name: "nested app asset", in: "app/overview/assets/index.js", want: "assets/index.js"},
		{name: "nested login asset", in: "login/assets/index.css", want: "assets/index.css"},
		{name: "not asset", in: "app/overview", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nestedFrontendAssetPath(test.in); got != test.want {
				t.Fatalf("nestedFrontendAssetPath(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestNestedEmbeddedAssetPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "root app", in: "app.js", want: "app.js"},
		{name: "nested app", in: "app/overview/app.js", want: "app.js"},
		{name: "nested css", in: "app/overview/styles.css", want: "styles.css"},
		{name: "other", in: "app/overview/logo.png", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nestedEmbeddedAssetPath(test.in); got != test.want {
				t.Fatalf("nestedEmbeddedAssetPath(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestServeFrontendFromDiskServesNestedAsset(t *testing.T) {
	frontendDir := t.TempDir()
	assetsDir := filepath.Join(frontendDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(frontendDir, "index.html"), []byte("<!doctype html><div id=\"root\"></div>"), 0o644); err != nil {
		t.Fatalf("WriteFile(index) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatalf("WriteFile(asset) error = %v", err)
	}

	server := &Server{opts: Options{FrontendDist: frontendDir}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/app/overview/assets/app.js", nil)

	if !server.serveFrontendFromDisk(recorder, request) {
		t.Fatalf("serveFrontendFromDisk() = false, want true")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "console.log('ok')") {
		t.Fatalf("body = %q, want asset content", body)
	}
}
