package ppcore

import (
	"context"
	"net"
	"time"

	"github.com/vakaka1/pp/internal/protocol"
	"github.com/vakaka1/pp/internal/proxy"
	"github.com/vakaka1/pp/internal/routing"
	"github.com/xtaci/smux"
	"go.uber.org/zap"
)

func ServeTunnelStream(stream *smux.Stream, log *zap.Logger) {
	serveTunnelStream(stream, log, nil)
}

func serveTunnelStream(stream *smux.Stream, log *zap.Logger, engine *routing.Engine) {
	defer stream.Close()

	var hdr protocol.PPStreamHeader
	if err := hdr.Decode(stream); err != nil {
		log.Debug("failed to decode stream header", zap.Error(err))
		return
	}

	target := hdr.AddressString()
	host, _, _ := net.SplitHostPort(target)
	ip := net.ParseIP(host)
	if ip == nil && engine != nil {
		ip = resolveTargetIP(host)
	}

	if engine != nil {
		policy := engine.Route(host, ip)
		if policy == routing.PolicyBlock {
			log.Debug("server-side routing blocked target", zap.String("target", target))
			_, _ = stream.Write([]byte{protocol.StatusConnRefused})
			return
		}
	}

	conn, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		stream.Write([]byte{protocol.StatusUnreachable})
		return
	}
	defer conn.Close()

	if _, err := stream.Write([]byte{protocol.StatusOK}); err != nil {
		return
	}

	errc := make(chan error, 2)
	go func() {
		_, err := proxy.Copy(conn, stream)
		errc <- err
	}()
	go func() {
		_, err := proxy.Copy(stream, conn)
		errc <- err
	}()
	<-errc
}

func resolveTargetIP(host string) net.IP {
	if host == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil
	}
	return ips[0]
}
