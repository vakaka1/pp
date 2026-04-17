package ppweb

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

//go:embed webui/*
var embeddedFrontend embed.FS

func (s *Server) serveFrontend(w http.ResponseWriter, r *http.Request) {
	if s.serveFrontendFromDisk(w, r) {
		return
	}
	s.serveEmbeddedFrontend(w, r)
}

func (s *Server) serveFrontendFromDisk(w http.ResponseWriter, r *http.Request) bool {
	indexPath := filepath.Join(s.opts.FrontendDist, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return false
	}

	requestPath := normalizeFrontendPath(r.URL.Path)
	if requestPath == "index.html" {
		http.ServeFile(w, r, indexPath)
		return true
	}

	targetPath := filepath.Join(s.opts.FrontendDist, filepath.FromSlash(requestPath))
	if info, err := os.Stat(targetPath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, targetPath)
		return true
	}

	http.ServeFile(w, r, indexPath)
	return true
}

func (s *Server) serveEmbeddedFrontend(w http.ResponseWriter, r *http.Request) {
	webUI, err := fs.Sub(embeddedFrontend, "webui")
	if err != nil {
		http.Error(w, "embedded frontend is unavailable", http.StatusInternalServerError)
		return
	}

	requestPath := normalizeFrontendPath(r.URL.Path)
	if requestPath != "index.html" {
		if _, err := fs.Stat(webUI, requestPath); err == nil {
			req := r.Clone(r.Context())
			req.URL.Path = "/" + requestPath
			http.FileServer(http.FS(webUI)).ServeHTTP(w, req)
			return
		}
	}

	index, err := fs.ReadFile(webUI, "index.html")
	if err != nil {
		http.Error(w, "embedded frontend is unavailable", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(index)
}

func normalizeFrontendPath(requestPath string) string {
	cleaned := path.Clean("/" + strings.TrimSpace(requestPath))
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." {
		return "index.html"
	}
	if strings.HasPrefix(cleaned, "..") {
		return "index.html"
	}
	return cleaned
}
