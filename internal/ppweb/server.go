package ppweb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/user/pp/internal/config"
	"github.com/user/pp/internal/crypto"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "ppweb_session"

type Server struct {
	opts      Options
	store     *Store
	protocols *protocolRegistry
	coreCmd   *exec.Cmd
	log       *zap.Logger
}

func NewServer(opts Options) (*Server, error) {
	if strings.TrimSpace(opts.ListenAddress) == "" {
		opts.ListenAddress = "0.0.0.0:4090"
	}
	if strings.TrimSpace(opts.DatabasePath) == "" {
		opts.DatabasePath = filepath.Join("pp-web-data", "pp-web.sqlite")
	}
	if strings.TrimSpace(opts.FrontendDist) == "" {
		opts.FrontendDist = filepath.Join("pp-web", "frontend", "dist")
	}
	if strings.TrimSpace(opts.CoreConfigPath) == "" {
		opts.CoreConfigPath = filepath.Join("pp-web-data", "generated", "pp-core.json")
	}
	if opts.SessionTTL == 0 {
		opts.SessionTTL = 14 * 24 * time.Hour
	}
	if strings.TrimSpace(opts.ProjectRoot) == "" {
		if wd, err := os.Getwd(); err == nil {
			opts.ProjectRoot = wd
		}
	}

	store, err := OpenStore(opts.DatabasePath)
	if err != nil {
		return nil, err
	}

	logger, _ := zap.NewProduction()

	return &Server{
		opts:      opts,
		store:     store,
		protocols: newProtocolRegistry(),
		log:       logger,
	}, nil
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	return s.store.Close()
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		s.serveAPI(w, r)
		return
	}
	s.serveFrontend(w, r)
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	switch {
	case r.URL.Path == "/api/bootstrap" && r.Method == http.MethodGet:
		s.handleBootstrap(w, r)
	case r.URL.Path == "/api/setup" && r.Method == http.MethodPost:
		s.handleSetup(w, r)
	case r.URL.Path == "/api/login" && r.Method == http.MethodPost:
		s.handleLogin(w, r)
	case r.URL.Path == "/api/logout" && r.Method == http.MethodPost:
		s.handleLogout(w, r)
	case r.URL.Path == "/api/protocols" && r.Method == http.MethodGet:
		s.withAdmin(w, r, s.handleProtocols)
	case r.URL.Path == "/api/overview" && r.Method == http.MethodGet:
		s.withAdmin(w, r, s.handleOverview)
	case r.URL.Path == "/api/connections" && r.Method == http.MethodGet:
		s.withAdmin(w, r, s.handleConnectionsList)
	case r.URL.Path == "/api/connections" && r.Method == http.MethodPost:
		s.withAdmin(w, r, s.handleConnectionCreate)
	case strings.HasPrefix(r.URL.Path, "/api/connections/") && strings.HasSuffix(r.URL.Path, "/clients"):
		s.withAdmin(w, r, s.handleClientsListOrCreate)
	case strings.HasPrefix(r.URL.Path, "/api/connections/") && strings.Contains(r.URL.Path, "/clients/") && strings.HasSuffix(r.URL.Path, "/config"):
		s.withAdmin(w, r, s.handleClientConfig)
	case strings.HasPrefix(r.URL.Path, "/api/connections/") && strings.HasSuffix(r.URL.Path, "/setup-https"):
		s.withAdmin(w, r, s.handleSetupHTTPS)
	case strings.HasPrefix(r.URL.Path, "/api/connections/") && strings.HasSuffix(r.URL.Path, "/nginx-config"):
		s.withAdmin(w, r, s.handleNginxConfig)
	case strings.HasPrefix(r.URL.Path, "/api/clients/"):
		s.withAdmin(w, r, s.handleClientDelete)
	case strings.HasPrefix(r.URL.Path, "/api/connections/"):
		s.withAdmin(w, r, s.handleConnectionRoute)
	case r.URL.Path == "/api/tools/generate-secrets" && r.Method == http.MethodPost:
		s.withAdmin(w, r, s.handleGenerateSecrets)
	case r.URL.Path == "/api/tools/check-port" && r.Method == http.MethodGet:
		s.withAdmin(w, r, s.handleCheckPort)
	case r.URL.Path == "/api/pp-core/sync" && r.Method == http.MethodPost:
		s.withAdmin(w, r, s.handleSyncCoreConfig)
	case r.URL.Path == "/api/pp-core/restart" && r.Method == http.MethodPost:
		s.withAdmin(w, r, s.handleRestartCore)
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}

type apiHandler func(http.ResponseWriter, *http.Request, *Admin)

func (s *Server) withAdmin(w http.ResponseWriter, r *http.Request, handler apiHandler) {
	admin, err := s.currentAdmin(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if admin == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	handler(w, r, admin)
}

func (s *Server) currentAdmin(r *http.Request) (*Admin, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		if err == http.ErrNoCookie {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read session cookie: %w", err)
	}
	return s.store.FindAdminBySession(r.Context(), cookie.Value)
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	setupRequired, err := s.setupRequired(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	settings, err := s.store.GetAppSettings(r.Context(), s.opts.CoreConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	admin, err := s.currentAdmin(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	publicIP := "Unknown"
	if ip, err := getPublicIP(); err == nil {
		publicIP = ip
	}

	response := map[string]any{
		"setupRequired": setupRequired,
		"authenticated": admin != nil,
		"appName":       settings.AppName,
		"build":         s.opts.Build,
		"publicIP":      publicIP,
		"listen":        s.opts.ListenAddress,
	}
	if admin != nil {
		response["user"] = map[string]any{
			"id":       admin.ID,
			"username": admin.Username,
		}
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if setupRequired, err := s.setupRequired(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !setupRequired {
		writeError(w, http.StatusConflict, "setup is already completed")
		return
	}

	var payload struct {
		AppName  string `json:"appName"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	payload.AppName = strings.TrimSpace(payload.AppName)
	payload.Username = strings.TrimSpace(payload.Username)
	if payload.AppName == "" {
		payload.AppName = "PP Web"
	}
	if payload.Username == "" {
		writeError(w, http.StatusBadRequest, "administrator username is required")
		return
	}
	if len(payload.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	if err := s.store.CreateInitialAdmin(
		r.Context(),
		payload.AppName,
		payload.Username,
		string(passwordHash),
		s.opts.CoreConfigPath,
	); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	admin, err := s.store.FindAdminByUsername(r.Context(), payload.Username)
	if err != nil || admin == nil {
		writeError(w, http.StatusInternalServerError, "failed to load created administrator")
		return
	}

	session, err := s.store.CreateSession(r.Context(), admin.ID, s.opts.SessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.setSessionCookie(w, session)

	writeJSON(w, http.StatusCreated, map[string]any{
		"user": map[string]any{
			"id":       admin.ID,
			"username": admin.Username,
		},
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if setupRequired, err := s.setupRequired(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if setupRequired {
		writeError(w, http.StatusConflict, "setup must be completed before login")
		return
	}

	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	admin, err := s.store.FindAdminByUsername(r.Context(), strings.TrimSpace(payload.Username))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if admin == nil || bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(payload.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	session, err := s.store.CreateSession(r.Context(), admin.ID, s.opts.SessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.setSessionCookie(w, session)

	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":       admin.ID,
			"username": admin.Username,
		},
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.store.DeleteSession(r.Context(), cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleProtocols(w http.ResponseWriter, r *http.Request, _ *Admin) {
	writeJSON(w, http.StatusOK, map[string]any{
		"protocols": s.protocols.Descriptors(),
	})
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request, _ *Admin) {
	settings, err := s.store.GetAppSettings(r.Context(), s.opts.CoreConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	connections, err := s.store.ListConnections(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Fetch client PSKs for all connections
	clientPSKsByConn := make(map[int64][]string)
	for _, conn := range connections {
		clients, _ := s.store.ListClientsByConnection(r.Context(), conn.ID)
		psks := make([]string, 0, len(clients))
		for _, c := range clients {
			if c.PSK != "" {
				psks = append(psks, c.PSK)
			}
		}
		clientPSKsByConn[conn.ID] = psks
	}

	cfg, buildErr := s.protocols.BuildCoreConfig(connections, clientPSKsByConn)
	configPreview := "{}"
	configValid := false
	if cfg != nil {
		if preview, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			configPreview = string(preview)
		}
		if err := cfg.Validate(true); err == nil {
			configValid = true
		}
	}

	protocolUsage := make(map[string]int)
	listenerCards := make([]map[string]any, 0, len(connections))
	reachableCount := 0
	activeConnections := 0
	for _, connection := range connections {
		protocolUsage[connection.Protocol]++
		if !connection.Enabled {
			listenerCards = append(listenerCards, map[string]any{
				"id":        connection.ID,
				"name":      connection.Name,
				"listen":    connection.Listen,
				"enabled":   false,
				"reachable": false,
				"protocol":  connection.Protocol,
			})
			continue
		}

		activeConnections++
		reachable := probeAddress(connection.Listen)
		if reachable {
			reachableCount++
		}
		listenerCards = append(listenerCards, map[string]any{
			"id":        connection.ID,
			"name":      connection.Name,
			"listen":    connection.Listen,
			"enabled":   true,
			"reachable": reachable,
			"protocol":  connection.Protocol,
		})
	}

	protocolCards := make([]map[string]any, 0, len(s.protocols.Descriptors()))
	for _, descriptor := range s.protocols.Descriptors() {
		protocolCards = append(protocolCards, map[string]any{
			"id":            descriptor.ID,
			"name":          descriptor.Name,
			"summary":       descriptor.Summary,
			"installed":     descriptor.Installed,
			"statusLabel":   descriptor.StatusLabel,
			"usageCount":    protocolUsage[descriptor.ID],
			"accent":        descriptor.Accent,
			"fieldsCount":   countFields(descriptor),
			"sectionsCount": len(descriptor.Sections),
		})
	}

	binaryStatus := s.inspectPPCoreBinary()

	if buildErr != nil {
		settings.LastSyncError = buildErr.Error()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"appName": settings.AppName,
		"summary": map[string]any{
			"connectionsTotal":   len(connections),
			"connectionsActive":  activeConnections,
			"protocolsInstalled": len(protocolCards),
			"listenersReachable": reachableCount,
		},
		"core": map[string]any{
			"embeddedReady":   true,
			"binaryAvailable": binaryStatus["available"],
			"binaryPath":      binaryStatus["path"],
			"binaryVersion":   binaryStatus["version"],
			"binaryError":     binaryStatus["error"],
			"configPath":      settings.CoreConfigPath,
			"configValid":     configValid,
			"configPreview":   configPreview,
			"lastSyncAt":      settings.LastSyncAt,
			"lastSyncError":   settings.LastSyncError,
		},
		"protocols":     protocolCards,
		"listeners":     listenerCards,
		"build":         s.opts.Build,
		"hasBuildError": buildErr != nil,
	})
}

func (s *Server) handleConnectionsList(w http.ResponseWriter, r *http.Request, _ *Admin) {
	connections, err := s.store.ListConnections(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"connections": connections,
		"protocols":   s.protocols.Descriptors(),
	})
}

func (s *Server) handleClientsListOrCreate(w http.ResponseWriter, r *http.Request, _ *Admin) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		writeError(w, http.StatusBadRequest, "invalid connection id")
		return
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid connection id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		clients, err := s.store.ListClientsByConnection(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Strip PSK from list – the client-specific config endpoint is used to download it.
		for i := range clients {
			clients[i].PSK = ""
		}
		writeJSON(w, http.StatusOK, map[string]any{"clients": clients})

	case http.MethodPost:
		var payload struct {
			Name string `json:"name"`
		}
		if err := decodeJSON(r, &payload); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		psk, err := crypto.GeneratePSK()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate client PSK")
			return
		}
		client, err := s.store.CreateClient(r.Context(), id, payload.Name, psk)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		warning := s.applyCoreOnlyAndSummarize(r.Context())
		writeJSON(w, http.StatusCreated, map[string]any{
			"client":  client,
			"warning": warning,
		})
	}
}

func (s *Server) handleSetupHTTPS(w http.ResponseWriter, r *http.Request, _ *Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid connection id")
		return
	}

	connection, err := s.store.GetConnection(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if connection == nil {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	}

	var payload struct {
		Mode string `json:"mode"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var domain string
	if d, ok := connection.Settings["domain"].(string); ok {
		domain = d
	}
	if domain == "" {
		writeError(w, http.StatusBadRequest, "connection has no domain configured")
		return
	}

	tlsConfig := &config.TLSConfig{Enabled: true}
	var stagedConnection *Connection
	revertStagedSite := false

	switch payload.Mode {
	case "self-signed":
		certDir := s.certDirectory(connection.Tag)
		certPath := filepath.Join(certDir, "cert.pem")
		keyPath := filepath.Join(certDir, "key.pem")
		if err := crypto.GenerateSelfSignedCert(domain, certPath, keyPath); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate cert: "+err.Error())
			return
		}
		tlsConfig.CertFile = certPath
		tlsConfig.KeyFile = keyPath

	case "lets-encrypt":
		s.log.Info("requesting lets-encrypt certificate", zap.String("domain", domain))
		if !s.serviceUnitExists("nginx") {
			writeError(w, http.StatusBadRequest, "nginx service is required for lets-encrypt mode")
			return
		}

		webrootDir := s.acmeChallengeDirectory(connection.Tag)
		if err := os.MkdirAll(webrootDir, 0o750); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to prepare ACME webroot: "+err.Error())
			return
		}

		staged := *connection
		staged.TLS = &config.TLSConfig{Enabled: true}
		stagedConnection = &staged
		if err := s.reconcileNginxConfigs(r.Context(), stagedConnection, connection); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to publish HTTP site before certbot: "+err.Error())
			return
		}
		revertStagedSite = true
		defer func() {
			if !revertStagedSite || stagedConnection == nil {
				return
			}
			rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer rollbackCancel()
			if err := s.reconcileNginxConfigs(rollbackCtx, connection, stagedConnection); err != nil {
				s.log.Warn("failed to roll back staged nginx config after certbot error", zap.Error(err), zap.String("domain", domain))
			}
		}()

		certbotCtx, certbotCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer certbotCancel()

		out, err := runPrivilegedCommand(
			certbotCtx,
			"certbot",
			"certonly",
			"--webroot",
			"--webroot-path",
			webrootDir,
			"--non-interactive",
			"--agree-tos",
			"--register-unsafely-without-email",
			"-d",
			domain,
		)
		if err != nil {
			s.log.Error("certbot failed", zap.Error(err), zap.String("output", string(out)))
			writeError(w, http.StatusInternalServerError, "Certbot failed: "+string(out))
			return
		}

		tlsConfig.CertFile = fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", domain)
		tlsConfig.KeyFile = fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", domain)

	default:
		writeError(w, http.StatusBadRequest, "invalid mode")
		return
	}

	input := ConnectionInput{
		Name:     connection.Name,
		Tag:      connection.Tag,
		Protocol: connection.Protocol,
		Listen:   connection.Listen,
		TLS:      tlsConfig,
		Enabled:  connection.Enabled,
		Settings: connection.Settings,
	}

	_, warning, err := s.persistConnection(r.Context(), id, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save connection: "+err.Error())
		return
	}
	revertStagedSite = false

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tls": tlsConfig, "warning": warning})
}

func (s *Server) handleNginxConfig(w http.ResponseWriter, r *http.Request, _ *Admin) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid connection id")
		return
	}

	connection, err := s.store.GetConnection(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if connection == nil {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	}

	config, err := s.buildNginxConfig(connection)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"config": config, "path": s.nginxSiteConfigPath(connection.Tag)})
}

func (s *Server) handleClientDelete(w http.ResponseWriter, r *http.Request, _ *Admin) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		writeError(w, http.StatusBadRequest, "invalid client id")
		return
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid client id")
		return
	}
	if err := s.store.DeleteClient(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "warning": s.applyCoreOnlyAndSummarize(r.Context())})
}

func (s *Server) handleConnectionCreate(w http.ResponseWriter, r *http.Request, _ *Admin) {
	payload, err := s.decodeConnectionInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	connection, warning, err := s.persistConnection(r.Context(), 0, payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, connectionMutationResponse{
		Connection: connection,
		Warning:    warning,
	})
}

func (s *Server) handleConnectionRoute(w http.ResponseWriter, r *http.Request, _ *Admin) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/connections/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "connection route not found")
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid connection id")
		return
	}

	if len(parts) == 2 && parts[1] == "client-config" && r.Method == http.MethodGet {
		s.handleConnectionClientConfig(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodPut:
		payload, err := s.decodeConnectionInput(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		connection, warning, err := s.persistConnection(r.Context(), id, payload)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, connectionMutationResponse{
			Connection: connection,
			Warning:    warning,
		})
	case http.MethodDelete:
		connection, err := s.store.GetConnection(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if connection == nil {
			writeError(w, http.StatusNotFound, "connection not found")
			return
		}
		if err := s.store.DeleteConnection(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		warning := s.applyAfterDeleteAndSummarize(r.Context(), connection)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"warning": warning,
		})
	case http.MethodGet:
		connection, err := s.store.GetConnection(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if connection == nil {
			writeError(w, http.StatusNotFound, "connection not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"connection": connection})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleConnectionClientConfig(w http.ResponseWriter, r *http.Request, id int64) {
	connection, err := s.store.GetConnection(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if connection == nil {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	}

	clientConfig, err := s.protocols.BuildClientConfig(*connection)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"connectionId": id,
		"config":       clientConfig,
	})
}

// handleClientConfig serves GET /api/connections/{connId}/clients/{clientId}/config
// It returns a ready-to-use pp client config that contains the specific client's PSK,
// so each client gets a unique config and can be individually revoked.
func (s *Server) handleClientConfig(w http.ResponseWriter, r *http.Request, _ *Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	// Path: /api/connections/{connId}/clients/{clientId}/config
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// parts: ["api","connections",connId,"clients",clientId,"config"]
	if len(parts) < 6 {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	connID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid connection id")
		return
	}
	clientID, err := strconv.ParseInt(parts[4], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid client id")
		return
	}

	connection, err := s.store.GetConnection(r.Context(), connID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if connection == nil {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	}

	client, err := s.store.GetClient(r.Context(), clientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if client == nil || client.ConnectionID != connID {
		writeError(w, http.StatusNotFound, "client not found")
		return
	}
	if client.PSK == "" {
		writeError(w, http.StatusInternalServerError, "client has no PSK — try deleting and re-creating this client")
		return
	}

	clientConfig, err := s.protocols.BuildClientConfigForClient(*connection, client.Name, client.PSK)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, clientConfig)
}

func (s *Server) handleGenerateSecrets(w http.ResponseWriter, r *http.Request, _ *Admin) {
	var payload struct {
		Protocol string `json:"protocol"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	secrets, err := s.protocols.GenerateSecrets(strings.TrimSpace(payload.Protocol))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"secrets": secrets})
}

func (s *Server) handleCheckPort(w http.ResponseWriter, r *http.Request, _ *Admin) {
	portStr := r.URL.Query().Get("port")
	if portStr == "" {
		writeError(w, http.StatusBadRequest, "port parameter is required")
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		writeError(w, http.StatusBadRequest, "invalid port number")
		return
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"port":      port,
			"available": false,
		})
		return
	}
	_ = ln.Close()

	writeJSON(w, http.StatusOK, map[string]any{
		"port":      port,
		"available": true,
	})
}

func (s *Server) handleSyncCoreConfig(w http.ResponseWriter, r *http.Request, _ *Admin) {
	warning := s.syncOnlyAndSummarize(r.Context())
	if warning != "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"warning": warning,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRestartCore(w http.ResponseWriter, r *http.Request, _ *Admin) {
	result, err := s.restartCore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "pid": result.PID, "method": result.Method, "output": result.Output})
}

func (s *Server) decodeConnectionInput(r *http.Request) (ConnectionInput, error) {
	var payload ConnectionInput
	if err := decodeJSON(r, &payload); err != nil {
		return ConnectionInput{}, err
	}
	return s.protocols.NormalizeConnection(payload)
}

func (s *Server) persistConnection(ctx context.Context, id int64, input ConnectionInput) (*Connection, string, error) {
	var previous *Connection
	var err error
	if id != 0 {
		previous, err = s.store.GetConnection(ctx, id)
		if err != nil {
			return nil, "", err
		}
	}

	connection, err := s.store.SaveConnection(ctx, id, input)
	if err != nil {
		return nil, "", err
	}

	warning := joinWarnings(connectionRuntimeWarning(connection), s.applyAndSummarize(ctx, connection, previous))
	return connection, warning, nil
}

func (s *Server) applyAndSummarize(ctx context.Context, connection *Connection, previous *Connection) string {
	if err := s.applyConnectionRuntime(ctx, connection, previous); err != nil {
		return err.Error()
	}
	return ""
}

func (s *Server) applyCoreOnlyAndSummarize(ctx context.Context) string {
	if err := s.applyCoreConfig(ctx); err != nil {
		return err.Error()
	}
	return ""
}

func (s *Server) applyAfterDeleteAndSummarize(ctx context.Context, deleted *Connection) string {
	if err := s.applyRuntimeAfterDelete(ctx, deleted); err != nil {
		return err.Error()
	}
	return ""
}

func (s *Server) syncOnlyAndSummarize(ctx context.Context) string {
	if _, err := s.syncCoreConfig(ctx); err != nil {
		return err.Error()
	}
	return ""
}

func (s *Server) syncCoreConfig(ctx context.Context) (bool, error) {
	settings, err := s.store.GetAppSettings(ctx, s.opts.CoreConfigPath)
	if err != nil {
		return false, err
	}

	connections, err := s.store.ListConnections(ctx)
	if err != nil {
		return false, err
	}

	// Fetch client PSKs for all connections
	clientPSKsByConn := make(map[int64][]string)
	for _, conn := range connections {
		clients, _ := s.store.ListClientsByConnection(ctx, conn.ID)
		psks := make([]string, 0, len(clients))
		for _, c := range clients {
			if c.PSK != "" {
				psks = append(psks, c.PSK)
			}
		}
		clientPSKsByConn[conn.ID] = psks
	}

	cfg, err := s.protocols.BuildCoreConfig(connections, clientPSKsByConn)
	if err != nil {
		_ = s.store.RecordSyncResult(ctx, time.Now().UTC(), err.Error())
		return false, err
	}

	if len(cfg.Inbounds) == 0 {
		if err := os.Remove(settings.CoreConfigPath); err != nil && !os.IsNotExist(err) {
			_ = s.store.RecordSyncResult(ctx, time.Now().UTC(), err.Error())
			return false, fmt.Errorf("failed to remove stale core config: %w", err)
		}
		if err := s.store.RecordSyncResult(ctx, time.Now().UTC(), ""); err != nil {
			return false, err
		}
		return false, nil
	}

	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		_ = s.store.RecordSyncResult(ctx, time.Now().UTC(), err.Error())
		return false, fmt.Errorf("failed to encode generated core config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(settings.CoreConfigPath), 0o755); err != nil {
		_ = s.store.RecordSyncResult(ctx, time.Now().UTC(), err.Error())
		return false, fmt.Errorf("failed to create core config directory: %w", err)
	}

	tmpPath := settings.CoreConfigPath + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o644); err != nil {
		_ = s.store.RecordSyncResult(ctx, time.Now().UTC(), err.Error())
		return false, fmt.Errorf("failed to write temporary core config: %w", err)
	}
	if err := os.Rename(tmpPath, settings.CoreConfigPath); err != nil {
		_ = s.store.RecordSyncResult(ctx, time.Now().UTC(), err.Error())
		return false, fmt.Errorf("failed to atomically replace core config: %w", err)
	}

	if err := cfg.Validate(true); err != nil {
		_ = s.store.RecordSyncResult(ctx, time.Now().UTC(), err.Error())
		return false, fmt.Errorf("generated core config is currently invalid: %w", err)
	}

	if err := s.store.RecordSyncResult(ctx, time.Now().UTC(), ""); err != nil {
		return false, err
	}

	return true, nil
}

func (s *Server) setupRequired(ctx context.Context) (bool, error) {
	hasSetup, err := s.store.HasSetup(ctx)
	if err != nil {
		return false, err
	}
	return !hasSetup, nil
}

func (s *Server) setSessionCookie(w http.ResponseWriter, session *Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.ID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) inspectPPCoreBinary() map[string]any {
	status := map[string]any{
		"available": false,
		"path":      "",
		"version":   "",
		"error":     "",
	}

	candidates := []string{
		filepath.Join(s.opts.ProjectRoot, "bin", "pp"),
	}
	if path, err := exec.LookPath("pp"); err == nil {
		candidates = append(candidates, path)
	}

	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}

		if _, err := os.Stat(candidate); err != nil {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

		out, err := exec.CommandContext(ctx, candidate, "version").CombinedOutput()
		cancel()
		status["available"] = err == nil
		status["path"] = candidate
		if len(out) > 0 {
			status["version"] = strings.TrimSpace(string(out))
		}
		if err != nil {
			status["error"] = err.Error()
		}
		return status
	}

	status["error"] = "pp binary not found in PATH or ./bin/pp"
	return status
}

func probeAddress(address string) bool {
	conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func countFields(descriptor ProtocolDescriptor) int {
	total := 0
	for _, section := range descriptor.Sections {
		total += len(section.Fields)
	}
	return total
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON payload: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

func getPublicIP() (string, error) {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(ip), nil
}
