//go:build linux

package fulltunnel

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"github.com/vakaka1/pp/internal/config"
)

const chainName = "PP_FULL_TUNNEL"

var bypassCIDRs = []string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"224.0.0.0/4",
	"240.0.0.0/4",
}


func Up(cfg *config.ClientConfig, transparentListen string, owner string) error {
	if cfg == nil {
		return fmt.Errorf("client config is required")
	}
	if transparentListen == "" {
		transparentListen = cfg.TransparentListen
	}
	if transparentListen == "" {
		return fmt.Errorf("transparent listen address is required")
	}

	_, portStr, err := net.SplitHostPort(transparentListen)
	if err != nil {
		return fmt.Errorf("invalid transparent listen address: %w", err)
	}
	redirectPort, err := strconv.Atoi(portStr)
	if err != nil || redirectPort < 1 || redirectPort > 65535 {
		return fmt.Errorf("invalid transparent listen port: %q", portStr)
	}

	serverIP, serverPort, err := resolveServerEndpoint(cfg.Server.Address)
	if err != nil {
		return err
	}

	if err := Down(); err != nil {
		return err
	}

	if err := runIptables("-t", "nat", "-N", chainName); err != nil {
		return err
	}

	if owner != "" {
		if err := runIptables("-t", "nat", "-A", chainName, "-m", "owner", "--uid-owner", owner, "-j", "RETURN"); err != nil {
			return err
		}
	}

	for _, cidr := range bypassCIDRs {
		if err := runIptables("-t", "nat", "-A", chainName, "-d", cidr, "-j", "RETURN"); err != nil {
			return err
		}
	}

	if err := runIptables(
		"-t", "nat", "-A", chainName,
		"-p", "tcp",
		"-d", serverIP+"/32",
		"--dport", strconv.Itoa(serverPort),
		"-j", "RETURN",
	); err != nil {
		return err
	}

	if err := runIptables(
		"-t", "nat", "-A", chainName,
		"-p", "tcp",
		"-j", "REDIRECT",
		"--to-ports", strconv.Itoa(redirectPort),
	); err != nil {
		return err
	}

	if err := runIptables("-t", "nat", "-A", "OUTPUT", "-p", "tcp", "-j", chainName); err != nil {
		return err
	}

	return nil
}


func Down() error {
	_ = runIptablesAllowFailure("-t", "nat", "-D", "OUTPUT", "-p", "tcp", "-j", chainName)
	_ = runIptablesAllowFailure("-t", "nat", "-F", chainName)
	_ = runIptablesAllowFailure("-t", "nat", "-X", chainName)
	return nil
}

func resolveServerEndpoint(address string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, fmt.Errorf("invalid client.server.address %q: %w", address, err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("invalid server port %q", portStr)
	}

	if ip := net.ParseIP(host); ip != nil {
		ip4 := ip.To4()
		if ip4 == nil {
			return "", 0, fmt.Errorf("IPv6 server addresses are not supported in full-tunnel mode yet")
		}
		return ip4.String(), port, nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return "", 0, fmt.Errorf("failed to resolve server host %q: %w", host, err)
	}
	for _, ip := range ips {
		if ip4 := ip.To4(); ip4 != nil {
			return ip4.String(), port, nil
		}
	}

	return "", 0, fmt.Errorf("server host %q has no IPv4 address; full-tunnel mode currently supports IPv4 only", host)
}

func runIptables(args ...string) error {
	cmd := exec.Command("iptables", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runIptablesAllowFailure(args ...string) error {
	cmd := exec.Command("iptables", args...)
	_, err := cmd.CombinedOutput()
	return err
}
