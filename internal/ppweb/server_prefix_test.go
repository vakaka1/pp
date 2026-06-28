package ppweb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestServeHTTPPanelPrefixProtectsUnprefixedRoutes(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "ppweb.sqlite"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	settings, err := store.GetAppSettings(context.Background(), filepath.Join(t.TempDir(), "pp-core.json"))
	if err != nil {
		t.Fatalf("GetAppSettings() error = %v", err)
	}
	settings.PanelPrefix = "/panel"
	if err := store.UpdateAppSettings(context.Background(), settings); err != nil {
		t.Fatalf("UpdateAppSettings() error = %v", err)
	}

	server := &Server{store: store}

	unprefixed := httptest.NewRecorder()
	server.ServeHTTP(unprefixed, httptest.NewRequest(http.MethodGet, "/api/bootstrap", nil))
	if unprefixed.Code != http.StatusNotFound {
		t.Fatalf("unprefixed API status = %d, want %d", unprefixed.Code, http.StatusNotFound)
	}

	prefixed := httptest.NewRecorder()
	server.ServeHTTP(prefixed, httptest.NewRequest(http.MethodGet, "/panel/api/bootstrap", nil))
	if prefixed.Code != http.StatusOK {
		t.Fatalf("prefixed API status = %d, want %d; body=%q", prefixed.Code, http.StatusOK, prefixed.Body.String())
	}
}
