package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/user/pp/internal/config"
	"github.com/user/pp/internal/fulltunnel"
	"github.com/user/pp/internal/ppcore"
	"github.com/user/pp/internal/routing"
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
)

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
			fmt.Printf("PP-Client Version: %s\nBuild Date: %s\nCommit: %s\n", version, buildDate, gitCommit)
		},
	}

	validateCmd := &cobra.Command{
		Use:   "validate-config",
		Short: "Validate client config",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.LoadConfig(cfgFile)
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
	validateCmd.MarkFlagRequired("config")

	clientCmd := &cobra.Command{
		Use:   "start",
		Short: "Start client proxy",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.LoadConfig(cfgFile)
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

			geoIpData, _ := os.ReadFile("data/geoip.dat")
			geoIpDB, _ := routing.LoadGeoIP(geoIpData)
			geoSiteData, _ := os.ReadFile("data/geosite.dat")
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
	clientCmd.MarkFlagRequired("config")

	fullTunnelCmd := &cobra.Command{
		Use:    "full-tunnel",
		Short:  "Manage Linux full-tunnel TCP redirection rules",
		Hidden: true,
	}

	fullTunnelUpCmd := &cobra.Command{
		Use:   "up",
		Short: "Enable full-tunnel TCP redirection",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.LoadConfig(cfgFile)
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
	fullTunnelUpCmd.Flags().StringVar(&fullTunnelOwner, "owner", "", "Username or UID to exempt from redirection")
	fullTunnelUpCmd.MarkFlagRequired("config")

	fullTunnelDownCmd := &cobra.Command{
		Use:   "down",
		Short: "Disable full-tunnel TCP redirection",
		Run: func(cmd *cobra.Command, args []string) {
			if err := fulltunnel.Down(); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
	}

	fullTunnelCmd.AddCommand(fullTunnelUpCmd, fullTunnelDownCmd)

	rootCmd.AddCommand(versionCmd, validateCmd, clientCmd, fullTunnelCmd)
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
