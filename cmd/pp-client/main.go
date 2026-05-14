package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/vakaka1/pp/internal/config"
	"github.com/vakaka1/pp/internal/fulltunnel"
	"github.com/vakaka1/pp/internal/ppcore"
	"github.com/vakaka1/pp/internal/routing"
	"github.com/vakaka1/pp/internal/sysproxy"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	version   = "dev"
	buildDate = "unknown"
	gitCommit = "none"

	cfgFile           string
	verbose           bool
	transparentListen string
	fullTunnelOwner   string
	enableSysProxy    bool
	enableFullTunnel  bool
)

var pingTimePattern = regexp.MustCompile(`time[=<]([0-9]+(?:[.,][0-9]+)?)\s*ms`)

type testResultLine struct {
	Status    string   `json:"status"`
	ConnectOK bool     `json:"connect_ok"`
	PingOK    bool     `json:"ping_ok"`
	PingMS    *float64 `json:"ping_ms"`
	Error     string   `json:"error,omitempty"`
}

func printTestResultLine(result testResultLine) {
	data, err := json.Marshal(result)
	if err != nil {
		fmt.Printf("PP_CLIENT_TEST_RESULT {\"status\":\"error\",\"connect_ok\":false,\"ping_ok\":false,\"ping_ms\":null,\"error\":\"failed to encode result\"}\n")
		return
	}
	fmt.Printf("PP_CLIENT_TEST_RESULT %s\n", data)
}

func dataFilePath(name string) string {
	if runtime.GOOS == "windows" {
		exePath, err := os.Executable()
		if err == nil {
			p := filepath.Join(filepath.Dir(exePath), "data", name)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		appData := os.Getenv("APPDATA")
		if appData != "" {
			p := filepath.Join(appData, "pp", "data", name)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return filepath.Join("data", name)
}

func pingTargetFromConfig(cfg *config.ClientConfig) string {
	if cfg.Server.Domain != "" {
		return cfg.Server.Domain
	}
	host, _, err := net.SplitHostPort(cfg.Server.Address)
	if err == nil {
		return host
	}
	return cfg.Server.Address
}

func pingSite(ctx context.Context, target string) (time.Duration, string, error) {
	args := []string{"-c", "1", "-W", "5", target}
	if runtime.GOOS == "windows" {
		args = []string{"-n", "1", "-w", "5000", target}
	}

	start := time.Now()
	out, err := exec.CommandContext(ctx, "ping", args...).CombinedOutput()
	elapsed := time.Since(start)
	output := strings.TrimSpace(string(out))
	if err != nil {
		return 0, output, err
	}

	matches := pingTimePattern.FindStringSubmatch(output)
	if len(matches) != 2 {
		return elapsed, output, nil
	}
	value := strings.ReplaceAll(matches[1], ",", ".")
	ms, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return elapsed, output, nil
	}
	return time.Duration(ms * float64(time.Millisecond)), output, nil
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "pp-client",
		Short: "PP Client",
	}
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable DEBUG logging")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("PP-Client Version: %s\nBuild Date: %s\nCommit: %s\nOS: %s/%s\n", version, buildDate, gitCommit, runtime.GOOS, runtime.GOARCH)
		},
	}

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Update pp-client using the official installer",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runSelfUpdate(); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
	}

	testCmd := &cobra.Command{
		Use:   "test [config-name]",
		Short: "Validate config and test server availability",
		Run: func(cmd *cobra.Command, args []string) {
			target := cfgFile
			if target == "" && len(args) > 0 {
				target = args[0]
			}
			resolvedPath, err := resolveConfigPath(target)
			if err != nil {
				fmt.Println("Error:", err)
				printTestResultLine(testResultLine{Status: "error", Error: err.Error()})
				os.Exit(1)
			}
			cfg, err := config.LoadConfig(resolvedPath)
			if err != nil {
				fmt.Println("Error loading config:", err)
				printTestResultLine(testResultLine{Status: "error", Error: err.Error()})
				os.Exit(1)
			}
			if err := cfg.Validate(false); err != nil {
				fmt.Println("Client config invalid:")
				fmt.Println("-", err)
				printTestResultLine(testResultLine{Status: "error", Error: err.Error()})
				os.Exit(1)
			}
			fmt.Println("Client config is valid.")

			log := initLog(cfg.Log, verbose)
			fmt.Println("Checking server availability (site readiness)...")

			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()

			if err := ppcore.TestConnection(ctx, cfg.Client, log); err != nil {
				fmt.Println("Server availability check failed:")
				fmt.Println("-", err)
				printTestResultLine(testResultLine{Status: "error", Error: err.Error()})
				os.Exit(1)
			}
			fmt.Println("Server is available and site is ready.")

			pingTarget := pingTargetFromConfig(cfg.Client)
			fmt.Printf("Pinging %s to measure latency...\n", pingTarget)
			pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
			sitePing, pingOutput, err := pingSite(pingCtx, pingTarget)
			pingCancel()
			if err != nil {
				fmt.Printf("Site ping failed (%s): %v\n", pingTarget, err)
				if pingOutput != "" {
					fmt.Println(pingOutput)
				}
				printTestResultLine(testResultLine{Status: "error", ConnectOK: true, Error: err.Error()})
				os.Exit(1)
			}
			pingMS := float64(sitePing.Microseconds()) / 1000
			fmt.Printf("Ping result (%s): %.2fms\n", pingTarget, pingMS)
			printTestResultLine(testResultLine{Status: "ok", ConnectOK: true, PingOK: true, PingMS: &pingMS})
		},
	}
	testCmd.Flags().StringVar(&cfgFile, "config", "", "Config file")

	validateCmd := &cobra.Command{
		Use:   "validate-config [config-name]",
		Short: "Validate client config",
		Run: func(cmd *cobra.Command, args []string) {
			target := cfgFile
			if target == "" && len(args) > 0 {
				target = args[0]
			}
			resolvedPath, err := resolveConfigPath(target)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			cfg, err := config.LoadConfig(resolvedPath)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if err := cfg.Validate(false); err != nil {
				fmt.Println("Client config invalid:")
				fmt.Println("-", err)
				os.Exit(1)
			}
			fmt.Println("Client config valid.")
		},
	}
	validateCmd.Flags().StringVar(&cfgFile, "config", "", "Config file")

	clientCmd := &cobra.Command{
		Use:   "start [config-name]",
		Short: "Start client proxy",
		Run: func(cmd *cobra.Command, args []string) {
			target := cfgFile
			if target == "" && len(args) > 0 {
				target = args[0]
			}
			resolvedPath, err := resolveConfigPath(target)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			cfg, err := config.LoadConfig(resolvedPath)
			if err != nil {
				panic(err)
			}
			if transparentListen != "" && cfg.Client != nil {
				cfg.Client.TransparentListen = transparentListen
			}
			if err := cfg.Validate(false); err != nil {
				panic(err)
			}
			log := initLog(cfg.Log, verbose)

			geoIpData, _ := os.ReadFile(dataFilePath("geoip.dat"))
			geoIpDB, _ := routing.LoadGeoIP(geoIpData)
			geoSiteData, _ := os.ReadFile(dataFilePath("geosite.dat"))
			geoSiteDB, _ := routing.LoadGeoSite(geoSiteData)

			var routingCfg config.RoutingConfig
			if cfg.Client.Routing != nil {
				routingCfg = *cfg.Client.Routing
			}
			engine, err := routing.NewEngine(routingCfg, geoIpDB, geoSiteDB)
			if err != nil {
				log.Fatal("failed to initialize routing engine", zap.Error(err))
			}

			cli := ppcore.NewClient(cfg.Client, log, engine)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if enableSysProxy && cfg.Client.HTTPProxyListen != "" {
				if err := sysproxy.Enable(cfg.Client.HTTPProxyListen); err != nil {
					log.Warn("failed to enable system proxy", zap.Error(err))
				} else {
					log.Info("system proxy enabled", zap.String("address", cfg.Client.HTTPProxyListen))
					defer func() {
						if err := sysproxy.Disable(); err != nil {
							log.Warn("failed to disable system proxy", zap.Error(err))
						} else {
							log.Info("system proxy disabled")
						}
					}()
				}
			}

			if enableFullTunnel {
				if runtime.GOOS == "windows" {
					if !ensureAdmin(log) {
						os.Exit(1)
					}
				}
				owner := fullTunnelOwner
				if runtime.GOOS == "linux" && owner == "" {
					owner = "root"
				}
				listenAddr := transparentListen
				if runtime.GOOS == "windows" {
					listenAddr = ""
					cfg.Client.TransparentListen = ""
				} else if listenAddr == "" {
					listenAddr = "127.0.0.1:1090"
					cfg.Client.TransparentListen = listenAddr
				}
				if err := fulltunnel.Up(cfg.Client, listenAddr, owner); err != nil {
					log.Fatal("failed to enable full-tunnel", zap.Error(err))
				}
				log.Info("full-tunnel enabled", zap.String("transparent_listen", listenAddr), zap.String("owner", owner))
				defer func() {
					if err := fulltunnel.Down(); err != nil {
						log.Warn("failed to disable full-tunnel", zap.Error(err))
					} else {
						log.Info("full-tunnel disabled")
					}
				}()
			}

			go func() {
				sig := make(chan os.Signal, 1)
				signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
				<-sig
				cancel()
			}()

			if err := cli.Start(ctx); err != nil {
				log.Fatal("client error", zap.Error(err))
			}
			<-ctx.Done()
		},
	}
	clientCmd.Flags().StringVar(&cfgFile, "config", "", "Config file")
	clientCmd.Flags().StringVar(&transparentListen, "transparent-listen", "", "Transparent TCP listener for redirected full-tunnel traffic")
	clientCmd.Flags().BoolVar(&enableSysProxy, "system-proxy", false, "Enable system proxy on start (Windows: registry, other: no-op)")
	clientCmd.Flags().BoolVar(&enableFullTunnel, "full-tunnel", false, "Enable full-tunnel mode (requires root/admin; Windows prompts automatically)")
	clientCmd.Flags().StringVar(&fullTunnelOwner, "owner", "", "Username or UID to exempt from full-tunnel redirection (Linux only)")

	fullTunnelCmd := &cobra.Command{
		Use:   "full-tunnel",
		Short: "Manage full-tunnel traffic redirection",
	}

	fullTunnelUpCmd := &cobra.Command{
		Use:   "up [config-name]",
		Short: "Enable full-tunnel redirection",
		Run: func(cmd *cobra.Command, args []string) {
			target := cfgFile
			if target == "" && len(args) > 0 {
				target = args[0]
			}
			resolvedPath, err := resolveConfigPath(target)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			cfg, err := config.LoadConfig(resolvedPath)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if transparentListen != "" && cfg.Client != nil {
				cfg.Client.TransparentListen = transparentListen
			}
			if err := cfg.Validate(false); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if runtime.GOOS == "windows" {
				// Temporary logger for ensureAdmin since fullTunnelUpCmd doesn't initialize a full logger
				logger, _ := zap.NewDevelopment()
				if !ensureAdmin(logger) {
					os.Exit(1)
				}
			}
			if err := fulltunnel.Up(cfg.Client, transparentListen, fullTunnelOwner); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
	}
	fullTunnelUpCmd.Flags().StringVar(&cfgFile, "config", "", "Config file")
	fullTunnelUpCmd.Flags().StringVar(&transparentListen, "transparent-listen", "", "Transparent TCP listener for redirected full-tunnel traffic")
	fullTunnelUpCmd.Flags().StringVar(&fullTunnelOwner, "owner", "", "Username or UID to exempt from redirection (Linux only)")

	fullTunnelDownCmd := &cobra.Command{
		Use:   "down",
		Short: "Disable full-tunnel redirection",
		Run: func(cmd *cobra.Command, args []string) {
			if err := fulltunnel.Down(); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
	}

	fullTunnelCmd.AddCommand(fullTunnelUpCmd, fullTunnelDownCmd)

	rootCmd.AddCommand(versionCmd, updateCmd, testCmd, validateCmd, clientCmd, fullTunnelCmd, importCmd, listCmd, deleteCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runSelfUpdate() error {
	switch runtime.GOOS {
	case "windows":
		err := startCommand("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "irm https://raw.githubusercontent.com/vakaka1/pp/main/scripts/install-client.ps1 | iex")
		if err == nil {
			fmt.Println("update started in PowerShell")
		}
		return err
	case "linux":
		if os.Geteuid() == 0 {
			return runCommand("bash", "-c", "curl -fsSL https://raw.githubusercontent.com/vakaka1/pp/main/scripts/install-client.sh | bash")
		}
		return runCommand("sudo", "bash", "-c", "curl -fsSL https://raw.githubusercontent.com/vakaka1/pp/main/scripts/install-client.sh | bash")
	default:
		return fmt.Errorf("update is not supported on %s", runtime.GOOS)
	}
}

func runCommand(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	return command.Run()
}

func startCommand(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	return command.Start()
}

func initLog(cfg config.LogConfig, verbose bool) *zap.Logger {
	level := zap.InfoLevel
	if verbose || cfg.Level == "debug" {
		level = zap.DebugLevel
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.AddSync(os.Stdout),
		level,
	)
	return zap.New(core)
}
