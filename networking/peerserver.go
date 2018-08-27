package networking

import (
	"encoding/json"
	"net/http"
)

var currentStore *ConnectionStore

func getPeers(w http.ResponseWriter, r *http.Request) {
	peers := []string{}

	for k := range currentStore.clients {
		peers = append(peers, k.conn.RemoteAddr().String())

		if len(peers) > 100 {
			break
		}
	}

	resp, _ := json.Marshal(peers)
	w.Write(resp)
}

// StartPeerServer creates an HTTP server that replies with known peers
func (cs *ConnectionStore) StartPeerServer() {
	http.HandleFunc("/peers", getPeers)

	currentStore = cs

	http.ListenAndServe(":80", nil)
}
