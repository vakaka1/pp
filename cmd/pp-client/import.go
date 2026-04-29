package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vakaka1/pp/internal/config"
)

var importCmd = &cobra.Command{
	Use:   "import [uri]",
	Short: "Import a client config from a ppf:// URI",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		uriStr := args[0]
		u, err := url.Parse(uriStr)
		if err != nil {
			fmt.Printf("Failed to parse URI: %v\n", err)
			os.Exit(1)
		}

		if u.Scheme != "ppf" {
			fmt.Printf("Unsupported scheme: %s\n", u.Scheme)
			os.Exit(1)
		}

		q := u.Query()
		proto := q.Get("proto")
		if proto != "pp-fallback" {
			fmt.Printf("Unsupported protocol: %s\n", proto)
			os.Exit(1)
		}

		clientName := u.User.Username()
		if clientName == "" {
			clientName = "client"
		}

		cfg := config.Config{
			Meta: &config.ConfigMeta{
				ClientName:  clientName,
				Protocol:    proto,
				GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			},
			Log: config.LogConfig{
				Level:  "info",
				Output: "stdout",
			},
			Client: &config.ClientConfig{
				Socks5Listen:      "127.0.0.1:1080",
				HTTPProxyListen:   "127.0.0.1:8080",
				TransparentListen: "",
			},
		}

		cfg.Client.Server.Address = u.Host
		cfg.Client.Server.Domain = strings.Split(u.Host, ":")[0]
		cfg.Client.Server.NoisePublicKey = q.Get("pub")
		cfg.Client.Server.PSK = q.Get("psk")
		cfg.Client.Server.TLSFingerprint = q.Get("fp")
		if cfg.Client.Server.TLSFingerprint == "" {
			cfg.Client.Server.TLSFingerprint = "chrome"
		}
		cfg.Client.Server.GRPCPath = q.Get("path")
		cfg.Client.Server.GRPCUserAgent = "grpc-go/1.62.1"
		cfg.Client.Transport.JitterMaxMs = 30
		cfg.Client.Transport.ShaperEnabled = true
		cfg.Client.Transport.KeepaliveIntervalSeconds = 25
		cfg.Client.Transport.ReconnectStreamMin = 800
		cfg.Client.Transport.ReconnectStreamMax = 1200
		cfg.Client.Transport.ReconnectDurationMinH = 3
		cfg.Client.Transport.ReconnectDurationMaxH = 5
		cfg.Client.ConnectionPool.Size = 1

		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			fmt.Printf("Failed to generate config JSON: %v\n", err)
			os.Exit(1)
		}

		fileName := clientName + ".json"
		var savePath string
		if _, err := os.Stat("/etc/pp"); err == nil {
			savePath = filepath.Join("/etc/pp", fileName)
		} else if _, err := os.Stat("configs"); err == nil {
			savePath = filepath.Join("configs", fileName)
		} else {
			savePath = fileName
		}

		if err := os.WriteFile(savePath, data, 0600); err != nil {
			fmt.Printf("Failed to write config to %s: %v\n", savePath, err)
			os.Exit(1)
		}

		fmt.Printf("Successfully imported config to %s\n", savePath)
	},
}
