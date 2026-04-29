package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/vakaka1/pp/internal/config"
	"github.com/vakaka1/pp/internal/crypto"
	"github.com/vakaka1/pp/internal/ppcore"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	version   = "dev"
	buildDate = "unknown"
	gitCommit = "none"

	cfgFile      string
	verbose      bool
	validateMode string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "pp-core",
		Short: "PP Core (Server Orchestrator)",
	}
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable DEBUG logging")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("PP-Core Version: %s\nBuild Date: %s\nCommit: %s\n", version, buildDate, gitCommit)
		},
	}

	keygenCmd := &cobra.Command{
		Use:   "keygen",
		Short: "Generate keys",
		Run: func(cmd *cobra.Command, args []string) {
			priv, pub, _ := crypto.GenerateX25519KeyPair()
			psk, _ := crypto.GeneratePSK()

			out := map[string]interface{}{
				"inbound_settings": map[string]string{
					"noise_private_key": priv,
					"psk":               psk,
				},
				"client_config": map[string]interface{}{
					"server": map[string]string{
						"noise_public_key": pub,
						"psk":              psk,
					},
				},
			}
			j, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(j))
		},
	}

	validateCmd := &cobra.Command{
		Use:   "validate-config",
		Short: "Validate server config",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.LoadConfig(cfgFile)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if err := cfg.Validate(true); err != nil {
				fmt.Println("Server/core config invalid:")
				fmt.Println("-", err)
				os.Exit(1)
			}
			fmt.Println("Server/core config valid.")
		},
	}
	validateCmd.Flags().StringVar(&cfgFile, "config", "", "Config file")
	validateCmd.MarkFlagRequired("config")

	generateClientCmd := &cobra.Command{
		Use:   "generate-client",
		Short: "Generate a client configuration from a core configuration",
		Run: func(cmd *cobra.Command, args []string) {
			srvCfg, err := config.LoadConfig(cfgFile)
			if err != nil {
				fmt.Println("Error reading server config:", err)
				os.Exit(1)
			}
			if err := srvCfg.Validate(true); err != nil {
				fmt.Println("Server config is invalid:", err)
				os.Exit(1)
			}

			if len(srvCfg.Inbounds) == 0 {
				fmt.Println("No inbounds found in server config.")
				os.Exit(1)
			}

			var fbSettings config.FallbackSettings
			json.Unmarshal(srvCfg.Inbounds[0].Settings, &fbSettings)

			pubKey, err := crypto.DerivePublicKey(fbSettings.NoisePrivateKey)
			if err != nil {
				fmt.Println("Failed to derive public key:", err)
				os.Exit(1)
			}

			cliCfg := config.Config{
				Log: config.LogConfig{
					Level:  "info",
					Output: "stdout",
				},
				Client: &config.ClientConfig{},
			}

			cliCfg.Client.Socks5Listen = "127.0.0.1:1080"
			cliCfg.Client.HTTPProxyListen = "127.0.0.1:8080"

			address := fbSettings.Domain
			cliCfg.Client.Server.Address = address + ":443"
			cliCfg.Client.Server.Domain = address
			cliCfg.Client.Server.NoisePublicKey = pubKey
			cliCfg.Client.Server.PSK = fbSettings.PSK
			cliCfg.Client.Server.TLSFingerprint = "chrome"
			cliCfg.Client.Server.GRPCPath = fbSettings.GRPCPath
			cliCfg.Client.Server.GRPCUserAgent = "grpc-go/1.62.1"

			cliCfg.Client.Transport.JitterMaxMs = 30
			cliCfg.Client.Transport.ShaperEnabled = true
			cliCfg.Client.Transport.KeepaliveIntervalSeconds = 25
			cliCfg.Client.Transport.ReconnectStreamMin = 800
			cliCfg.Client.Transport.ReconnectStreamMax = 1200
			cliCfg.Client.Transport.ReconnectDurationMinH = 3
			cliCfg.Client.Transport.ReconnectDurationMaxH = 5
			cliCfg.Client.ConnectionPool.Size = 1

			j, _ := json.MarshalIndent(cliCfg, "", "  ")
			fmt.Println(string(j))
		},
	}
	generateClientCmd.Flags().StringVar(&cfgFile, "config", "/etc/pp/server.json", "Server config file path to read from")

	serverCmd := &cobra.Command{
		Use:   "core",
		Short: "Start PP Core Orchestrator",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.LoadConfig(cfgFile)
			if err != nil {
				panic(err)
			}
			if err := cfg.Validate(true); err != nil {
				panic(err)
			}
			log := initLog(cfg.Log, verbose)

			core, err := ppcore.NewCore(cfg, log)
			if err != nil {
				log.Fatal("failed to initialize core", zap.Error(err))
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go func() {
				sig := make(chan os.Signal, 1)
				signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
				<-sig
				cancel()
			}()

			if err := core.Start(ctx); err != nil {
				log.Fatal("core error", zap.Error(err))
			}
		},
	}
	serverCmd.Flags().StringVar(&cfgFile, "config", "", "Config file")
	serverCmd.MarkFlagRequired("config")

	rootCmd.AddCommand(versionCmd, keygenCmd, validateCmd, generateClientCmd, serverCmd)
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
