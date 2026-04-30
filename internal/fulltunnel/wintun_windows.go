//go:build windows

package fulltunnel

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"github.com/vakaka1/pp/internal/config"
)

func Up(cfg *config.ClientConfig, transparentListen string, owner string) error {
	if cfg == nil {
		return fmt.Errorf("client config is required")
	}

	serverIP, _, err := resolveServerEndpoint(cfg.Server.Address)
	if err != nil {
		return err
	}

	if err := Down(); err != nil {
		return err
	}

	defaultGateway, defaultIfIndex, err := getDefaultRoute()
	if err != nil {
		return fmt.Errorf("cannot detect default gateway: %w", err)
	}

	cmds := [][]string{
		{"route", "add", serverIP, "mask", "255.255.255.255", defaultGateway, "metric", "1", "if", defaultIfIndex},
		{"route", "add", "0.0.0.0", "mask", "128.0.0.0", defaultGateway, "metric", "999", "if", defaultIfIndex},
		{"route", "add", "128.0.0.0", "mask", "128.0.0.0", defaultGateway, "metric", "999", "if", defaultIfIndex},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			_ = Down()
			return fmt.Errorf("route command %q failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}

	proxyAddr := cfg.HTTPProxyListen
	if proxyAddr == "" {
		proxyAddr = "127.0.0.1:8080"
	}

	if err := enableSystemProxy(proxyAddr); err != nil {
		_ = Down()
		return fmt.Errorf("failed to enable system proxy: %w", err)
	}

	return nil
}

func Down() error {
	disableSystemProxy()

	cmds := [][]string{
		{"route", "delete", "0.0.0.0", "mask", "128.0.0.0"},
		{"route", "delete", "128.0.0.0", "mask", "128.0.0.0"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		_ = cmd.Run()
	}
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
			return "", 0, fmt.Errorf("IPv6 not supported in full-tunnel mode")
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

	return "", 0, fmt.Errorf("server host %q has no IPv4 address", host)
}

func getDefaultRoute() (string, string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		"$r = Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Sort-Object RouteMetric | Select-Object -First 1; Write-Output \"$($r.NextHop)|$($r.InterfaceIndex)\"")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("powershell Get-NetRoute failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 2)
	if len(parts) != 2 || net.ParseIP(parts[0]) == nil {
		return "", "", fmt.Errorf("unexpected gateway output: %q", strings.TrimSpace(string(out)))
	}
	return parts[0], parts[1], nil
}

func enableSystemProxy(httpAddr string) error {
	script := fmt.Sprintf(`
Set-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' -Name ProxyEnable -Value 1
Set-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' -Name ProxyServer -Value '%s'
Set-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' -Name ProxyOverride -Value 'localhost;127.*;10.*;192.168.*;<local>'
`, httpAddr)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("powershell proxy setup failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func disableSystemProxy() {
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		"Set-ItemProperty -Path 'HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings' -Name ProxyEnable -Value 0")
	_ = cmd.Run()
}
