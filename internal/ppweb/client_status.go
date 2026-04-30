package ppweb

import (
	"encoding/json"
	"os"
	"strconv"
	"time"
)

type runtimeClientStatusFile struct {
	Clients map[string]runtimeClientStatus `json:"clients"`
}

type runtimeClientStatus struct {
	Online   bool      `json:"online"`
	LastSeen time.Time `json:"lastSeen"`
}

func (s *Server) enrichClientsWithRuntimeStatus(clients []Client) error {
	raw, err := os.ReadFile(s.clientStatusPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var status runtimeClientStatusFile
	if err := json.Unmarshal(raw, &status); err != nil {
		return err
	}

	for i := range clients {
		entry, ok := status.Clients[clientStatusKey(clients[i].ID)]
		if !ok {
			continue
		}
		clients[i].Online = entry.Online
		if !entry.LastSeen.IsZero() {
			lastSeen := entry.LastSeen
			clients[i].LastSeen = &lastSeen
		}
	}
	return nil
}

func clientStatusKey(id int64) string {
	return strconv.FormatInt(id, 10)
}
