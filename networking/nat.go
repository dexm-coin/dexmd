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
	ips, err := GetPeerList(cs.network)
	if err != nil {
		return err
	}

	for _, i := range ips {
		cs.Connect(i)
	}

	return nil
}

// GetPeerList returns other nodes in the network
func GetPeerList(network string) ([]string, error) {
	peerURL := fmt.Sprintf("https://%s.dexm.space/peers", network)

	resp, err := http.Get(peerURL)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	ipsDuplicate := []string{}
	err = json.Unmarshal(data, &ipsDuplicate)
	if err != nil {
		return nil, err
	}

	if len(ipsDuplicate) == 0 {
		ipsDuplicate = append(ipsDuplicate, "35.211.241.218")
	}

	var ips []string
	for _, ip1 := range ipsDuplicate {
		duplicate := false
		for _, ip2 := range ips {
			if ip2 == ip1 {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		ips = append(ips, ip1)
	}

	return ips, err
}
