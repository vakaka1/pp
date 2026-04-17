package protocol

import (
	"bytes"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// H2Settings represent the Chrome-like HTTP/2 settings.
func GetChromeSettings() []http2.Setting {
	return []http2.Setting{
		{ID: http2.SettingHeaderTableSize, Val: 65536},
		{ID: http2.SettingEnablePush, Val: 0},
		{ID: http2.SettingMaxConcurrentStreams, Val: 1000},
		{ID: http2.SettingInitialWindowSize, Val: 6291456}, // 6 MB
		{ID: http2.SettingMaxHeaderListSize, Val: 262144},
	}
}

// GenerateGRPCClientHeaders generates the pseudo-headers for the gRPC request.
func GenerateGRPCClientHeaders(domain, path, jwtToken, userAgent string) []hpack.HeaderField {
	return []hpack.HeaderField{
		{Name: ":method", Value: "POST"},
		{Name: ":scheme", Value: "https"},
		{Name: ":path", Value: path},
		{Name: ":authority", Value: domain},
		{Name: "content-type", Value: "application/grpc"},
		{Name: "te", Value: "trailers"},
		{Name: "user-agent", Value: userAgent},
		{Name: "authorization", Value: "Bearer " + jwtToken},
		{Name: "grpc-encoding", Value: "identity"},
		{Name: "grpc-accept-encoding", Value: "gzip,identity"},
	}
}

// GenerateGRPCServerHeaders generates the response headers for a valid gRPC connection.
func GenerateGRPCServerHeaders() []hpack.HeaderField {
	return []hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "application/grpc"},
		{Name: "grpc-encoding", Value: "identity"},
	}
}

// WriteHeaders writes HPACK-encoded headers.
func WriteHeaders(framer *http2.Framer, streamID uint32, endStream bool, headers []hpack.HeaderField) error {
	var buf bytes.Buffer
	encoder := hpack.NewEncoder(&buf)
	for _, h := range headers {
		_ = encoder.WriteField(h)
	}

	return framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: buf.Bytes(),
		EndStream:     endStream,
		EndHeaders:    true,
	})
}
