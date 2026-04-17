package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/flynn/noise"
)

type NoiseConn struct {
	net.Conn
	sendCipher *noise.CipherState
	recvCipher *noise.CipherState
	readBuf    bytes.Buffer
}

func NewNoiseConn(conn net.Conn, sendCipher, recvCipher *noise.CipherState) *NoiseConn {
	return &NoiseConn{
		Conn:       conn,
		sendCipher: sendCipher,
		recvCipher: recvCipher,
	}
}

func (c *NoiseConn) Read(b []byte) (n int, err error) {
	if c.readBuf.Len() > 0 {
		return c.readBuf.Read(b)
	}

	payload, err := ReadGRPCFrame(c.Conn)
	if err != nil {
		return 0, err
	}

	if len(payload) < 2 {
		return 0, fmt.Errorf("invalid noise frame length")
	}

	ciphertext := payload[2:]
	plaintext, err := c.recvCipher.Decrypt(nil, nil, ciphertext)
	if err != nil {
		return 0, fmt.Errorf("decrypt error: %w", err)
	}

	c.readBuf.Write(plaintext)
	return c.readBuf.Read(b)
}

func (c *NoiseConn) Write(b []byte) (n int, err error) {
	const maxPlaintext = 16384 - 16 - 2 // Max GRPC payload (16384) minus MAC (16) minus len prefix (2)

	for len(b) > 0 {
		chunkSize := len(b)
		if chunkSize > maxPlaintext {
			chunkSize = maxPlaintext
		}
		
		chunk := b[:chunkSize]
		ciphertext, err := c.sendCipher.Encrypt(nil, nil, chunk)
		if err != nil {
			return n, err
		}

		noiseFrame := make([]byte, 2+len(ciphertext))
		binary.BigEndian.PutUint16(noiseFrame[:2], uint16(len(ciphertext)))
		copy(noiseFrame[2:], ciphertext)

		if err := WriteGRPCFrame(c.Conn, noiseFrame); err != nil {
			return n, err
		}
		
		b = b[chunkSize:]
		n += chunkSize
	}

	return n, nil
}

// HttpConn wraps http Request Body and Response Writer into net.Conn
type HttpConn struct {
	R io.Reader
	W io.Writer
}

func (c *HttpConn) Read(b []byte) (n int, err error) {
	return c.R.Read(b)
}

func (c *HttpConn) Write(b []byte) (n int, err error) {
	n, err = c.W.Write(b)
	if flusher, ok := c.W.(interface{ Flush() }); ok {
		flusher.Flush()
	}
	return n, err
}

func (c *HttpConn) Close() error {
	if closer, ok := c.R.(io.Closer); ok {
		closer.Close()
	}
	return nil
}
func (c *HttpConn) LocalAddr() net.Addr { return &net.TCPAddr{} }
func (c *HttpConn) RemoteAddr() net.Addr { return &net.TCPAddr{} }
func (c *HttpConn) SetDeadline(t time.Time) error { return nil }
func (c *HttpConn) SetReadDeadline(t time.Time) error { return nil }
func (c *HttpConn) SetWriteDeadline(t time.Time) error { return nil }
