package protocol

import (
	"bytes"
	"testing"
)

func TestGRPCFrameCodec(t *testing.T) {
	payload := []byte("hello grpc world")
	buf := new(bytes.Buffer)

	err := WriteGRPCFrame(buf, payload)
	if err != nil {
		t.Fatalf("WriteGRPCFrame failed: %v", err)
	}

	if buf.Len() != GRPCHeaderSize+len(payload) {
		t.Fatalf("unexpected buffer length: %d", buf.Len())
	}

	readPayload, err := ReadGRPCFrame(buf)
	if err != nil {
		t.Fatalf("ReadGRPCFrame failed: %v", err)
	}

	if string(readPayload) != string(payload) {
		t.Fatalf("payload mismatch: expected %s, got %s", string(payload), string(readPayload))
	}
}

func TestGRPCFrameCodecEmptyPayload(t *testing.T) {
	payload := []byte{}
	buf := new(bytes.Buffer)

	err := WriteGRPCFrame(buf, payload)
	if err != nil {
		t.Fatalf("WriteGRPCFrame failed: %v", err)
	}

	readPayload, err := ReadGRPCFrame(buf)
	if err != nil {
		t.Fatalf("ReadGRPCFrame failed: %v", err)
	}

	if len(readPayload) != 0 {
		t.Fatalf("payload mismatch: expected empty, got %d bytes", len(readPayload))
	}
}

func TestGRPCFrameCodecTooLarge(t *testing.T) {
	// Create a header with length > 16384
	buf := new(bytes.Buffer)
	header := []byte{0, 0, 0, 0x40, 0x01} // length = 16385
	buf.Write(header)

	_, err := ReadGRPCFrame(buf)
	if err == nil {
		t.Fatalf("expected error for too large payload, got nil")
	}
}
