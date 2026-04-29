package ppweb

import (
	"time"

	"github.com/vakaka1/pp/internal/config"
)

type BuildInfo struct {
	Version   string `json:"version"`
	BuildDate string `json:"buildDate"`
	GitCommit string `json:"gitCommit"`
}

type Options struct {
	ListenAddress  string
	DatabasePath   string
	FrontendDist   string
	CoreConfigPath string
	ProjectRoot    string
	SessionTTL     time.Duration
	Build          BuildInfo
}

type Admin struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Session struct {
	ID        string
	AdminID   int64
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Connection struct {
	ID        int64             `json:"id"`
	Name      string            `json:"name"`
	Tag       string            `json:"tag"`
	Protocol  string            `json:"protocol"`
	Listen    string            `json:"listen"`
	TLS       *config.TLSConfig `json:"tls,omitempty"`
	Enabled   bool              `json:"enabled"`
	Settings  map[string]any    `json:"settings"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

type ConnectionInput struct {
	Name     string            `json:"name"`
	Tag      string            `json:"tag"`
	Protocol string            `json:"protocol"`
	Listen   string            `json:"listen"`
	TLS      *config.TLSConfig `json:"tls,omitempty"`
	Enabled  bool              `json:"enabled"`
	Settings map[string]any    `json:"settings"`
}

type Client struct {
	ID           int64  `json:"id"`
	ConnectionID int64  `json:"connection_id"`
	Name         string `json:"name"`
	// PSK is the client's unique pre-shared key, returned only on creation and config download.
	PSK       string    `json:"psk,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	Online    bool      `json:"online"`
	BytesUsed int64     `json:"bytesUsed"`
	LastSeen  time.Time `json:"lastSeen,omitempty"`
}

type AppSettings struct {
	AppName        string    `json:"appName"`
	CoreConfigPath string    `json:"coreConfigPath"`
	LastSyncAt     time.Time `json:"lastSyncAt"`
	LastSyncError  string    `json:"lastSyncError"`
	InitializedAt  time.Time `json:"initializedAt"`
}

type ProtocolDescriptor struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Summary     string            `json:"summary"`
	StatusLabel string            `json:"statusLabel"`
	Accent      string            `json:"accent"`
	Installed   bool              `json:"installed"`
	Sections    []ProtocolSection `json:"sections"`
}

type ProtocolSection struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Fields      []ProtocolField `json:"fields"`
}

type ProtocolField struct {
	Path        string           `json:"path"`
	Label       string           `json:"label"`
	Kind        string           `json:"kind"`
	Required    bool             `json:"required"`
	Sensitive   bool             `json:"sensitive,omitempty"`
	Placeholder string           `json:"placeholder,omitempty"`
	Help        string           `json:"help,omitempty"`
	Default     any              `json:"default,omitempty"`
	Min         *float64         `json:"min,omitempty"`
	Max         *float64         `json:"max,omitempty"`
	Step        *float64         `json:"step,omitempty"`
	Options     []ProtocolOption `json:"options,omitempty"`
}

type ProtocolOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type connectionMutationResponse struct {
	Connection *Connection `json:"connection,omitempty"`
	Warning    string      `json:"warning,omitempty"`
}
