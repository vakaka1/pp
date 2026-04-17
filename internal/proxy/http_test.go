package proxy

import (
	"bytes"
	"strings"
	"testing"
)

func TestHandleHTTPConnect(t *testing.T) {
	conn := &mockConn{
		readBuf:  bytes.NewBufferString("CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n"),
		writeBuf: new(bytes.Buffer),
	}

	request, err := HandleHTTP(conn)
	if err != nil {
		t.Fatalf("HandleHTTP returned error: %v", err)
	}
	if request.Target != "example.com:443" {
		t.Fatalf("unexpected target: %s", request.Target)
	}
	if got := conn.writeBuf.String(); !strings.Contains(got, "200 Connection established") {
		t.Fatalf("expected CONNECT acknowledgement, got %q", got)
	}
}

func TestHandleHTTPForwardProxyRequest(t *testing.T) {
	raw := strings.Join([]string{
		"GET http://example.com/path?q=1 HTTP/1.1",
		"Host: example.com",
		"Proxy-Connection: keep-alive",
		"",
		"",
	}, "\r\n")

	conn := &mockConn{
		readBuf:  bytes.NewBufferString(raw),
		writeBuf: new(bytes.Buffer),
	}

	request, err := HandleHTTP(conn)
	if err != nil {
		t.Fatalf("HandleHTTP returned error: %v", err)
	}
	if request.Target != "example.com:80" {
		t.Fatalf("unexpected target: %s", request.Target)
	}
	payload := string(request.InitialData)
	if !strings.HasPrefix(payload, "GET /path?q=1 HTTP/1.1\r\n") {
		t.Fatalf("unexpected rewritten request line: %q", payload)
	}
	if strings.Contains(strings.ToLower(payload), "proxy-connection") {
		t.Fatalf("proxy-only headers should be stripped: %q", payload)
	}
}
