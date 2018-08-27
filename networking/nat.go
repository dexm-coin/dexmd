package networking

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/NebulousLabs/go-upnp"
)

// TraverseNat opens the port passed as an arguemnt and returns
// ip:port in a string. Required manaually closing port
func TraverseNat(port uint16, desc string) (string, error) {
	// Connect to router
	d, err := upnp.Discover()
	if err != nil {
		return "", err
	}

	ip, err := d.ExternalIP()
	if err != nil {
		return "", err
	}

	// Remove forwarding map and ignore error in case it doesn't exist
	d.Clear(port)

	// Open the port
	err = d.Forward(port, desc)

	return fmt.Sprintf("%s:%d", ip, port), nil
}

// FindPeers tries to find all peers for the selected network
func (cs *ConnectionStore) FindPeers() error {
	peerURL := fmt.Sprintf("https://%s.dexm.space/peers", cs.network)

	resp, err := http.Get(peerURL)
	if err != nil {
		return err
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	ips := []string{}
	err = json.Unmarshal(data, &ips)
	if err != nil {
		return err
	}

	if len(ips) == 0 {
		ips = append(ips, "35.231.132.159:3141")
	}

	for _, i := range ips {
		cs.Connect(i)
	}

	return nil
}
