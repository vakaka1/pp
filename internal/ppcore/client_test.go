package ppcore

import (
	"context"
	"errors"
	"testing"

	"github.com/vakaka1/pp/internal/config"
	"github.com/xtaci/smux"
	"go.uber.org/zap"
)

func TestTestConnectionReturnsConnectError(t *testing.T) {
	wantErr := errors.New("dial failed")
	original := testConnectToServer
	testConnectToServer = func(context.Context, *config.ClientConfig, *browserNoiseRunner) (*smux.Session, error) {
		return nil, wantErr
	}
	defer func() {
		testConnectToServer = original
	}()

	err := TestConnection(context.Background(), &config.ClientConfig{}, zap.NewNop())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected connect error %q, got %v", wantErr, err)
	}
}
