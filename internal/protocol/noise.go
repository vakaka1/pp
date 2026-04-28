package protocol

import (
	"crypto/sha256"
	"fmt"
	"net"
	"time"

	"github.com/flynn/noise"
)

// NoiseConfig configures a Noise_NK_25519_ChaChaPoly_BLAKE2s connection.
type NoiseConfig struct {
	StaticKeypair noise.DHKey
	ServerPublic  []byte // Client only
	IsClient      bool
	ServerDomain  string
}

// PerformNoiseNKHandshake runs the Noise_NK handshake.
func PerformNoiseNKHandshake(conn net.Conn, cfg *NoiseConfig) (*noise.CipherState, *noise.CipherState, error) {
	suite := noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashBLAKE2s)

	now := time.Now().Unix()
	prologueData := fmt.Sprintf("pp-v1%s%d", cfg.ServerDomain, now/3600)
	prologueHash := sha256.Sum256([]byte(prologueData))

	var hs *noise.HandshakeState
	var err error
	var sendCipher, recvCipher *noise.CipherState

	if cfg.IsClient {
		hs, err = noise.NewHandshakeState(noise.Config{
			CipherSuite: suite,
			Random:      nil,
			Pattern:     noise.HandshakeNK,
			Initiator:   true,
			Prologue:    prologueHash[:],
			PeerStatic:  cfg.ServerPublic,
		})
		if err != nil {
			return nil, nil, err
		}

		// Message 1
		msg1, _, _, err := hs.WriteMessage(nil, nil)
		if err != nil {
			return nil, nil, err
		}
		if err := WriteGRPCFrame(conn, msg1); err != nil {
			return nil, nil, err
		}

		// Message 2
		msg2, err := ReadGRPCFrame(conn)
		if err != nil {
			return nil, nil, err
		}
		_, sendCipher, recvCipher, err = hs.ReadMessage(nil, msg2)
		if err != nil {
			return nil, nil, err
		}

	} else {
		hs, err = noise.NewHandshakeState(noise.Config{
			CipherSuite:   suite,
			Random:        nil,
			Pattern:       noise.HandshakeNK,
			Initiator:     false,
			Prologue:      prologueHash[:],
			StaticKeypair: cfg.StaticKeypair,
		})
		if err != nil {
			return nil, nil, err
		}

		// Message 1
		msg1, err := ReadGRPCFrame(conn)
		if err != nil {
			return nil, nil, err
		}
		_, _, _, err = hs.ReadMessage(nil, msg1)
		if err != nil {
			return nil, nil, err
		}

		// Message 2
		var msg2 []byte
		msg2, recvCipher, sendCipher, err = hs.WriteMessage(nil, nil)
		if err != nil {
			return nil, nil, err
		}
		if err := WriteGRPCFrame(conn, msg2); err != nil {
			return nil, nil, err
		}
	}

	return sendCipher, recvCipher, nil
}
