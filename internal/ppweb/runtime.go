package ppweb

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

type coreRestartResult struct {
	Method string
	Output string
	PID    int
}

func (s *Server) applyCoreConfig(ctx context.Context) error {
	hasConfig, err := s.syncCoreConfig(ctx)
	if err != nil {
		return err
	}
	if !hasConfig {
		return s.stopCore()
	}

	_, err = s.restartCore()
	return err
}

func (s *Server) applyConnectionRuntime(ctx context.Context, connection *Connection, previous *Connection) error {
	if err := s.applyCoreConfig(ctx); err != nil {
		return err
	}
	return s.reconcileNginxConfigs(ctx, connection, previous)
}

func (s *Server) applyRuntimeAfterDelete(ctx context.Context, deleted *Connection) error {
	if err := s.applyCoreConfig(ctx); err != nil {
		return err
	}
	return s.reconcileNginxConfigs(ctx, nil, deleted)
}

func (s *Server) restartCore() (*coreRestartResult, error) {
	if s.serviceUnitExists("pp-core") {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		out, err := runPrivilegedCommand(ctx, "systemctl", "restart", "pp-core")
		if err == nil {
			return &coreRestartResult{
				Method: "systemctl",
				Output: strings.TrimSpace(string(out)),
			}, nil
		}
		s.log.Warn("systemctl restart failed, falling back to direct run", zap.Error(err), zap.String("output", strings.TrimSpace(string(out))))
	}

	if s.coreCmd != nil && s.coreCmd.Process != nil {
		_ = s.coreCmd.Process.Kill()
	}

	binaryStatus := s.inspectPPCoreBinary()
	if !binaryStatus["available"].(bool) {
		return nil, fmt.Errorf("pp binary not available: %s", binaryStatus["error"].(string))
	}

	cmd := exec.Command(binaryStatus["path"].(string), "core", "--config", s.opts.CoreConfigPath)
	if s.opts.ProjectRoot != "" {
		cmd.Dir = s.opts.ProjectRoot
		logFile := filepath.Join("/var/log/pp", "pp-core.log")
		if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err == nil {
			if f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
				cmd.Stdout = f
				cmd.Stderr = f
			}
		}
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start pp-core: %w", err)
	}

	s.coreCmd = cmd
	return &coreRestartResult{
		Method: "direct",
		PID:    cmd.Process.Pid,
	}, nil
}

func (s *Server) stopCore() error {
	if s.serviceUnitExists("pp-core") {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		out, err := runPrivilegedCommand(ctx, "systemctl", "stop", "pp-core")
		if err == nil {
			return nil
		}
		s.log.Warn("systemctl stop failed, falling back to direct shutdown", zap.Error(err), zap.String("output", strings.TrimSpace(string(out))))
	}

	if s.coreCmd != nil && s.coreCmd.Process != nil {
		if err := s.coreCmd.Process.Kill(); err != nil && !strings.Contains(err.Error(), "process already finished") {
			return err
		}
		s.coreCmd = nil
	}
	return nil
}

func (s *Server) serviceUnitExists(name string) bool {
	unitPaths := []string{
		filepath.Join("/etc/systemd/system", name+".service"),
		filepath.Join("/lib/systemd/system", name+".service"),
		filepath.Join("/usr/lib/systemd/system", name+".service"),
	}
	for _, unitPath := range unitPaths {
		if _, err := os.Stat(unitPath); err == nil {
			return true
		}
	}
	return false
}

func (s *Server) certDirectory(tag string) string {
	return filepath.Join(filepath.Dir(s.opts.DatabasePath), "certs", tag)
}

func (s *Server) nginxSitesDirectory() string {
	return "/etc/nginx/pp-sites"
}

func (s *Server) nginxManagedIncludePath() string {
	return "/etc/nginx/conf.d/pp-managed.conf"
}

func (s *Server) nginxSiteConfigPath(tag string) string {
	tag = sanitizeFileFragment(tag)
	if tag == "" {
		tag = "connection"
	}
	return filepath.Join(s.nginxSitesDirectory(), "pp-"+tag+".conf")
}

func sanitizeFileFragment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

func (s *Server) shouldManageNginx(connection *Connection) bool {
	if connection == nil || !connection.Enabled {
		return false
	}
	if connection.TLS == nil || !connection.TLS.Enabled {
		return false
	}

	addr, err := net.ResolveTCPAddr("tcp", connection.Listen)
	if err != nil || addr == nil {
		return false
	}

	if addr.IP != nil && addr.IP.IsLoopback() {
		return true
	}

	return addr.Port != 443
}

func (s *Server) buildNginxConfig(connection *Connection) (string, error) {
	if connection == nil {
		return "", fmt.Errorf("connection is required")
	}

	domain, _ := connection.Settings["domain"].(string)
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return "", fmt.Errorf("connection has no domain")
	}
	if connection.TLS == nil || !connection.TLS.Enabled {
		return "", fmt.Errorf("connection HTTPS is not configured")
	}

	corePort := strings.TrimPrefix(connection.Listen, ":")
	if addr, err := net.ResolveTCPAddr("tcp", connection.Listen); err == nil && addr != nil && addr.Port != 0 {
		corePort = strconv.Itoa(addr.Port)
	}
	grpcPath, _ := connection.Settings["grpc_path"].(string)
	if grpcPath == "" {
		grpcPath = "/pp.v1.TunnelService/Connect"
	}

	var backendSSL strings.Builder
	httpUpstream := "http"
	grpcUpstream := "grpc"
	if connection.TLS != nil && connection.TLS.Enabled {
		httpUpstream = "https"
		grpcUpstream = "grpcs"
		backendSSL.WriteString(`
        proxy_ssl_server_name on;
        proxy_ssl_verify off;
        grpc_ssl_server_name on;
        grpc_ssl_verify off;`)
	}

	config := fmt.Sprintf(`# Managed by pp-web for connection %s
server {
    listen 80;
    listen 443 ssl http2;
    server_name %s;

    ssl_certificate %s;
    ssl_certificate_key %s;

    location / {
        proxy_pass %s://127.0.0.1:%s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
%s
        proxy_buffering off;
        proxy_read_timeout 3600s;
    }

    location %s {
        grpc_pass %s://127.0.0.1:%s;
        grpc_set_header Host $host;
        grpc_set_header X-Real-IP $remote_addr;
%s
        grpc_read_timeout 3600s;
        grpc_send_timeout 3600s;
    }
}
`, connection.Tag, domain, connection.TLS.CertFile, connection.TLS.KeyFile, httpUpstream, corePort, backendSSL.String(), grpcPath, grpcUpstream, corePort, backendSSL.String())

	return config, nil
}

func (s *Server) reconcileNginxConfigs(ctx context.Context, current *Connection, previous *Connection) error {
	if !s.serviceUnitExists("nginx") {
		return nil
	}

	type fileState struct {
		exists bool
		data   []byte
	}

	snapshots := map[string]fileState{}
	paths := map[string]struct{}{}
	if current != nil {
		paths[s.nginxSiteConfigPath(current.Tag)] = struct{}{}
	}
	if previous != nil {
		paths[s.nginxSiteConfigPath(previous.Tag)] = struct{}{}
	}

	for path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				snapshots[path] = fileState{}
				continue
			}
			return fmt.Errorf("failed to snapshot nginx config %s: %w", path, err)
		}
		snapshots[path] = fileState{exists: true, data: data}
	}

	restore := func() {
		for path, snapshot := range snapshots {
			if snapshot.exists {
				_ = os.WriteFile(path, snapshot.data, 0o640)
			} else {
				_ = os.Remove(path)
			}
		}
	}

	if previous != nil && (current == nil || current.Tag != previous.Tag || !s.shouldManageNginx(current)) {
		if err := os.Remove(s.nginxSiteConfigPath(previous.Tag)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove nginx config for connection %s: %w", previous.Tag, err)
		}
	}

	if s.shouldManageNginx(current) {
		config, err := s.buildNginxConfig(current)
		if err != nil {
			restore()
			return err
		}
		if err := os.MkdirAll(s.nginxSitesDirectory(), 0o750); err != nil {
			restore()
			return fmt.Errorf("failed to create nginx managed directory: %w", err)
		}
		if err := os.WriteFile(s.nginxSiteConfigPath(current.Tag), []byte(config), 0o640); err != nil {
			restore()
			return fmt.Errorf("failed to write nginx config: %w", err)
		}
	} else if current != nil {
		if err := os.Remove(s.nginxSiteConfigPath(current.Tag)); err != nil && !os.IsNotExist(err) {
			restore()
			return fmt.Errorf("failed to remove nginx config for connection %s: %w", current.Tag, err)
		}
	}

	if err := s.validateAndReloadNginx(ctx); err != nil {
		restore()
		_ = s.validateAndReloadNginx(ctx)
		return err
	}

	return nil
}

func (s *Server) validateAndReloadNginx(ctx context.Context) error {
	nginxBinary, err := exec.LookPath("nginx")
	if err != nil {
		nginxBinary = "/usr/sbin/nginx"
	}

	validateCtx, validateCancel := context.WithTimeout(ctx, 20*time.Second)
	defer validateCancel()
	out, err := runPrivilegedCommand(validateCtx, nginxBinary, "-t")
	if err != nil {
		return fmt.Errorf("nginx -t failed: %s", strings.TrimSpace(string(out)))
	}

	reloadCtx, reloadCancel := context.WithTimeout(ctx, 20*time.Second)
	defer reloadCancel()
	out, err = runPrivilegedCommand(reloadCtx, "systemctl", "reload", "nginx")
	if err != nil {
		return fmt.Errorf("failed to reload nginx: %s", strings.TrimSpace(string(out)))
	}

	return nil
}

func runPrivilegedCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := privilegedCommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

func privilegedCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	if os.Geteuid() == 0 {
		return exec.CommandContext(ctx, name, args...)
	}

	if sudoPath, err := exec.LookPath("sudo"); err == nil {
		sudoArgs := append([]string{"-n", name}, args...)
		return exec.CommandContext(ctx, sudoPath, sudoArgs...)
	}

	return exec.CommandContext(ctx, name, args...)
}

func connectionRuntimeWarning(connection *Connection) string {
	if connection == nil {
		return ""
	}

	addr, err := net.ResolveTCPAddr("tcp", connection.Listen)
	if err != nil {
		return ""
	}

	port := 0
	if addr != nil {
		port = addr.Port
	}
	tlsEnabled := connection.TLS != nil && connection.TLS.Enabled

	if !tlsEnabled {
		return "generated client configs expect HTTPS on :443; enable HTTPS and pp-web will publish this connection through nginx automatically"
	}
	if addr != nil && addr.IP != nil && addr.IP.IsLoopback() {
		return ""
	}
	if port != 443 {
		return ""
	}

	return ""
}

func joinWarnings(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, " ")
}
