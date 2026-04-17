package protocol

import (
	"net"
	"testing"

	"github.com/flynn/noise"
)

func TestNoiseNKHandshake(t *testing.T) {
	// Generate server static key
	kp, _ := noise.DH25519.GenerateKeypair(nil)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientCfg := &NoiseConfig{
		IsClient:     true,
		ServerPublic: kp.Public,
		ServerDomain: "example.com",
	}

	serverCfg := &NoiseConfig{
		IsClient:      false,
		StaticKeypair: kp,
		ServerDomain:  "example.com",
	}

	errCh := make(chan error, 2)
	var clientSend, clientRecv, serverSend, serverRecv *noise.CipherState

	go func() {
		var err error
		clientSend, clientRecv, err = PerformNoiseNKHandshake(clientConn, clientCfg)
		errCh <- err
	}()

	go func() {
		var err error
		serverSend, serverRecv, err = PerformNoiseNKHandshake(serverConn, serverCfg)
		errCh <- err
	}()

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("handshake error: %v", err)
		}
	}

	if clientSend == nil || serverRecv == nil {
		t.Fatalf("cipher states are nil")
	}

	// Test Transport Frames (encryption)
	msg := []byte("secret message")
	ciphertext, _ := clientSend.Encrypt(nil, nil, msg)
	plaintext, _ := serverRecv.Decrypt(nil, nil, ciphertext)

	if string(plaintext) != string(msg) {
		t.Fatalf("decrypted message mismatch")
	}

	// Just to use the variables
	_ = clientRecv
	_ = serverSend
}
