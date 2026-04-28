package ppweb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestHandleSetupHTTPSRejectsSelfSignedMode(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "ppweb.sqlite"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	connection, err := store.SaveConnection(context.Background(), 0, ConnectionInput{
		Name:     "test",
		Tag:      "test",
		Protocol: "pp-fallback",
		Listen:   ":8081",
		Enabled:  true,
		Settings: map[string]any{
			"domain": "example.com",
		},
	})
	if err != nil {
		t.Fatalf("SaveConnection() error = %v", err)
	}

	server := &Server{store: store}
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/connections/"+strconv.FormatInt(connection.ID, 10)+"/setup-https",
		strings.NewReader(`{"mode":"self-signed"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	server.handleSetupHTTPS(recorder, request, &Admin{})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("handleSetupHTTPS() status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "only lets-encrypt mode is supported") {
		t.Fatalf("handleSetupHTTPS() body = %q, want lets-encrypt only error", body)
	}
}
