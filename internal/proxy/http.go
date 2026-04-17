package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// HandleHTTP handles an HTTP CONNECT proxy request.
func HandleHTTP(conn net.Conn) (*Request, error) {
	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		return nil, err
	}

	target := req.Host
	if !strings.Contains(target, ":") {
		target += defaultHTTPPort(req)
	}

	if req.Method == http.MethodConnect {
		if _, err := conn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n")); err != nil {
			return nil, err
		}
		return &Request{Target: target}, nil
	}

	sanitizeProxyHeaders(req)
	req.RequestURI = ""

	var payload bytes.Buffer
	if err := req.Write(&payload); err != nil {
		return nil, fmt.Errorf("failed to encode upstream request: %w", err)
	}

	if buffered := reader.Buffered(); buffered > 0 {
		trailing := make([]byte, buffered)
		if _, err := io.ReadFull(reader, trailing); err != nil {
			return nil, fmt.Errorf("failed to read buffered proxy payload: %w", err)
		}
		payload.Write(trailing)
	}

	return &Request{
		Target:      target,
		InitialData: payload.Bytes(),
	}, nil
}

func defaultHTTPPort(req *http.Request) string {
	if strings.EqualFold(req.URL.Scheme, "https") {
		return ":443"
	}
	return ":80"
}

func sanitizeProxyHeaders(req *http.Request) {
	hopHeaders := []string{
		"Proxy-Connection",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Connection",
		"Keep-Alive",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	}
	for _, header := range hopHeaders {
		req.Header.Del(header)
	}
}
