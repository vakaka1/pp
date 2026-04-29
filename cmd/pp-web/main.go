package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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

	log.Printf("pp-web v%s (built %s) listening on http://%s", version, buildDate, *listenAddress)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("pp-web failed: %v", err)
	}
}
