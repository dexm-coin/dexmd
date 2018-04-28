package networking

import (
	"strconv"

	"github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/network"
	"github.com/gogo/protobuf/proto"
)

var (
	bc       *blockchain.Blockchain
	identity *wallet.Wallet
)

func (cs *ConnectionStore) handleMessage(pb *protobufs.Request) []byte {
	switch pb.GetType() {
	// GET_BLOCKCHAIN_LEN returns the current block index
	case protobufs.Request_GET_BLOCKCHAIN_LEN:
		return []byte(strconv.FormatUint(bc.CurrentBlock, 10))

	case protobufs.Request_GET_PEERS:
		peers := []string{}

		for k := range cs.clients {
			peers = append(peers, k.conn.RemoteAddr().String())
		}

		resp := &protobufs.Peers{
			Ip: peers,
		}

		b, err := proto.Marshal(resp)
		if err != nil {
			return []byte("Error")
		}

		return b

	}

	return []byte{}
}
