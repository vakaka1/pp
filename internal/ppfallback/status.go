package ppfallback

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type clientStatusStore struct {
	path string
}

type clientStatusFile struct {
	Clients map[string]clientStatusEntry `json:"clients"`
}

type clientStatusEntry struct {
	Online    bool      `json:"online"`
	LastSeen  time.Time `json:"lastSeen"`
	BytesUsed int64     `json:"bytesUsed"`
}

var clientStatusLocks sync.Map

func newClientStatusStore(path string) *clientStatusStore {
	if path == "" {
		return nil
	}
	return &clientStatusStore{path: path}
}

func (s *clientStatusStore) Mark(clientID int64, online bool) {
	s.mark(clientID, online, true)
}

func (s *clientStatusStore) MarkOffline(clientID int64) {
	s.mark(clientID, false, false)
}

func (s *clientStatusStore) AddBytes(clientID int64, bytes int64) {
	if s == nil || clientID <= 0 || bytes <= 0 {
		return
	}

	mu := statusLockForPath(s.path)
	mu.Lock()
	defer mu.Unlock()

	status := clientStatusFile{Clients: map[string]clientStatusEntry{}}
	if raw, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(raw, &status)
	}
	if status.Clients == nil {
		status.Clients = map[string]clientStatusEntry{}
	}

	key := strconv.FormatInt(clientID, 10)
	entry := status.Clients[key]
	entry.BytesUsed += bytes
	status.Clients[key] = entry

	raw, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return
	}
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmpPath, s.path)
}

func (s *clientStatusStore) mark(clientID int64, online bool, touchLastSeen bool) {
	if s == nil || clientID <= 0 {
		return
	}

	mu := statusLockForPath(s.path)
	mu.Lock()
	defer mu.Unlock()

	status := clientStatusFile{Clients: map[string]clientStatusEntry{}}
	if raw, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(raw, &status)
	}
	if status.Clients == nil {
		status.Clients = map[string]clientStatusEntry{}
	}

	key := strconv.FormatInt(clientID, 10)
	entry := status.Clients[key]
	entry.Online = online
	if touchLastSeen {
		entry.LastSeen = time.Now().UTC()
	}
	status.Clients[key] = entry

	raw, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return
	}
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmpPath, s.path)
}

func statusLockForPath(path string) *sync.Mutex {
	value, _ := clientStatusLocks.LoadOrStore(path, &sync.Mutex{})
	return value.(*sync.Mutex)
}
