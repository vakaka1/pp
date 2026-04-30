package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

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
)

func configSearchDirs() []string {
	var dirs []string
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData != "" {
			dirs = append(dirs, filepath.Join(appData, "pp"))
		}
		exePath, err := os.Executable()
		if err == nil {
			dirs = append(dirs, filepath.Dir(exePath))
		}
	} else {
		dirs = append(dirs, "/etc/pp")
	}
	dirs = append(dirs, "configs")
	return dirs
}

func resolveConfigPath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("config name or path is required")
	}

	if info, err := os.Stat(name); err == nil && !info.IsDir() {
		return name, nil
	}

	var candidates []string
	searchDirs := configSearchDirs()

	if !strings.HasSuffix(name, ".json") {
		nameExt := name + ".json"
		candidates = append(candidates, nameExt)
		for _, dir := range searchDirs {
			candidates = append(candidates, filepath.Join(dir, nameExt))
		}
	} else {
		for _, dir := range searchDirs {
			candidates = append(candidates, filepath.Join(dir, name))
		}
	}

	for _, cand := range candidates {
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return cand, nil
		}
	}

	return "", fmt.Errorf("config file not found for: %s", name)
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

	rootCmd.AddCommand(versionCmd, validateCmd, clientCmd, fullTunnelCmd, importCmd, listCmd, deleteCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
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
