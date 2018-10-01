package networking

import (
	"encoding/json"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
)

var currentStore *ConnectionStore

func getPeers(w http.ResponseWriter, r *http.Request) {
	peers := []string{}

	for k := range currentStore.clients {
		fullIP := k.conn.RemoteAddr().String()

		ip := strings.Split(fullIP, ":")[0]
		peers = append(peers, ip)
	}

	resp, _ := json.Marshal(peers)
	w.Write(resp)
}

// StartPeerServer creates an HTTP server that replies with known peers
func (cs *ConnectionStore) StartPeerServer() {
	http.HandleFunc("/peers", getPeers)

	currentStore = cs

	log.Fatal(http.ListenAndServe(":80", nil))
}
