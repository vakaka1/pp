package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/vakaka1/pp/internal/ppweb"
)

var (
	version   = "dev"
	buildDate = "unknown"
	gitCommit = "none"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "apply-release" {
		if err := ppweb.RunReleaseApplyCommand(os.Args[2:]); err != nil {
			log.Fatalf("pp-web apply-release failed: %v", err)
		}
		return
	}

	defaultRoot, _ := os.Getwd()

	listenAddress := flag.String("listen", "127.0.0.1:4090", "HTTP address for PP Web")
	databasePath := flag.String("db", filepath.Join("pp-web-data", "pp-web.sqlite"), "Path to sqlite database")
	frontendDist := flag.String("frontend-dist", filepath.Join("pp-web", "frontend", "dist"), "Path to built Vite frontend")
	coreConfigPath := flag.String("core-config", filepath.Join("pp-web-data", "generated", "pp-core.json"), "Path to generated pp-core config")
	projectRoot := flag.String("project-root", defaultRoot, "Project root used for pp-core binary detection")
	flag.Parse()

	server, err := ppweb.NewServer(ppweb.Options{
		ListenAddress:  *listenAddress,
		DatabasePath:   *databasePath,
		FrontendDist:   *frontendDist,
		CoreConfigPath: *coreConfigPath,
		ProjectRoot:    *projectRoot,
		SessionTTL:     14 * 24 * time.Hour,
		Build: ppweb.BuildInfo{
			Version:   version,
			BuildDate: buildDate,
			GitCommit: gitCommit,
		},
	})
	if err != nil {
		log.Fatalf("failed to initialize pp-web: %v", err)
	}
	defer server.Close()

	// Load settings to override defaults
	settings, err := server.GetAppSettings(context.Background())
	if err != nil {
		log.Printf("warning: failed to load app settings: %v", err)
	} else {
		if settings.PanelPort != 0 {
			host, _, _ := net.SplitHostPort(*listenAddress)
			if host == "" {
				host = "0.0.0.0"
			}
			*listenAddress = net.JoinHostPort(host, strconv.Itoa(settings.PanelPort))
		}
	}

	httpServer := &http.Server{
		Addr:              *listenAddress,
		Handler:           server,
		ReadHeaderTimeout: 5 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		_ = httpServer.Close()
	}()

	if settings != nil && settings.PanelHTTPS {
		certFile := settings.PanelCertFile
		keyFile := settings.PanelKeyFile
		if certFile == "" || keyFile == "" {
			// Generate self-signed if not configured
			certDir := filepath.Join(filepath.Dir(*databasePath), "certs")
			certFile = filepath.Join(certDir, "panel.crt")
			keyFile = filepath.Join(certDir, "panel.key")
			domain := settings.PanelDomain
			if domain == "" {
				domain = "localhost"
			}
			log.Printf("generating self-signed certificate for %s", domain)
			if err := ppweb.GeneratePanelCert(domain, certFile, keyFile); err != nil {
				log.Fatalf("failed to generate self-signed cert: %v", err)
			}
		}
		log.Printf("pp-web v%s (built %s) listening on https://%s", version, buildDate, *listenAddress)
		if err := httpServer.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
			log.Fatalf("pp-web failed: %v", err)
		}
	} else {
		log.Printf("pp-web v%s (built %s) listening on http://%s", version, buildDate, *listenAddress)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("pp-web failed: %v", err)
		}
	}
}
