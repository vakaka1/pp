package proxy

// Request describes the upstream target requested by a local proxy client.
// InitialData contains bytes that must be forwarded to the upstream
// connection before switching to raw bidirectional relay.
type Request struct {
	Target      string
	InitialData []byte
}
