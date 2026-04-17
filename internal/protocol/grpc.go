package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

const GRPCHeaderSize = 5

// ReadGRPCFrame reads a standard gRPC 5-byte header and the subsequent message payload.
func ReadGRPCFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, GRPCHeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("failed to read grpc header: %w", err)
	}

	compressed := header[0]
	if compressed != 0 {
		return nil, fmt.Errorf("grpc message compression not supported (flag=%d)", compressed)
	}

	length := binary.BigEndian.Uint32(header[1:5])
	if length > 16384 {
		return nil, fmt.Errorf("grpc message too large: %d > 16384", length)
	}

	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, fmt.Errorf("failed to read grpc payload: %w", err)
		}
	}

	return payload, nil
}

// WriteGRPCFrame writes a standard gRPC 5-byte header followed by the payload.
func WriteGRPCFrame(w io.Writer, payload []byte) error {
	length := uint32(len(payload))
	header := make([]byte, GRPCHeaderSize)
	header[0] = 0 // Compression flag = 0
	binary.BigEndian.PutUint32(header[1:5], length)

	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("failed to write grpc header: %w", err)
	}

	if length > 0 {
		if _, err := w.Write(payload); err != nil {
			return fmt.Errorf("failed to write grpc payload: %w", err)
		}
	}
	return nil
}
