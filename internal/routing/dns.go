package routing

import (
	"context"
	"net"
)

// DNSResolver resolves domains based on routing policy.
type DNSResolver struct {
	strategy     string
	localServers []string
	remoteDialer func(context.Context, string, string) (net.Conn, error)
}

func NewDNSResolver(strategy string, localServers []string, dialer func(context.Context, string, string) (net.Conn, error)) *DNSResolver {
	return &DNSResolver{
		strategy:     strategy,
		localServers: localServers,
		remoteDialer: dialer,
	}
}

func (d *DNSResolver) Resolve(ctx context.Context, host string) ([]net.IP, error) {
	// A real implementation would handle different strategies here.
	// For "remote_for_proxied", it would check the routing engine first.
	// We'll fall back to net.LookupIP for simplicity in this mockup.
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}
