package networking

import (
	"context"

	"github.com/dexm-coin/dexmd/wallet"
	"github.com/libp2p/go-libp2p-peer"
	"github.com/libp2p/go-libp2p-peerstore"
	"github.com/libp2p/go-libp2p-swarm"
	bhost "github.com/libp2p/go-libp2p/p2p/host/basic"
	ma "github.com/multiformats/go-multiaddr"
)

type Node struct {
	host *bhost.BasicHost
}

// InitNode takes a Dexm wallet as identity and starts a node
func InitNode(identity *wallet.Wallet) (*Node, error) {
	ctx := context.Background()
	ps := peerstore.NewPeerstore()
	listen, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/3141")
	if err != nil {
		return nil, err
	}

	wal, err := identity.GetWallet()
	if err != nil {
		return nil, err
	}

	pid, err := peer.IDFromString(wal)
	if err != nil {
		return nil, err
	}

	net, err := swarm.NewNetwork(ctx, []ma.Multiaddr{listen}, pid, ps, nil)
	if err != nil {
		return nil, err
	}

	opts := bhost.HostOpts{
		EnableRelay: true,
	}

	host, err := bhost.NewHost(ctx, net, &opts)
	if err != nil {
		return nil, err
	}

	newNode := Node{host}
	return &newNode, nil
}
