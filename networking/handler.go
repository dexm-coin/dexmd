package networking

import (
	"fmt"
	"strconv"
	"time"

	protobufs "github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
)

func (cs *ConnectionStore) handleMessage(pb *protobufs.Request, c *client) []byte {
	switch pb.GetType() {
	// GET_BLOCKCHAIN_LEN returns the current block index
	case protobufs.Request_GET_BLOCKCHAIN_LEN:
		return []byte(strconv.FormatUint(cs.bc.CurrentBlock, 10))

	// GET_PEERS returns the peers the node is currently connected to
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

	// GET_BLOCK returns a block at the passed index
	case protobufs.Request_GET_BLOCK:
		block, err := cs.bc.GetBlock(pb.GetIndex())
		if err != nil {
			return []byte("Error")
		}

		return block

	// GET_WALLET_STATUS returns the current balance and nonce of a wallet
	case protobufs.Request_GET_WALLET_STATUS:
		walletAddr, err := c.GetResponse(100 * time.Millisecond)
		if err != nil {
			log.Error(err)
			return []byte{}
		}

		state, err := cs.bc.GetWalletState(fmt.Sprintf("%s", walletAddr))
		if err != nil {
			return []byte("Error")
		}

		data, err := proto.Marshal(&state)
		if err != nil {
			return []byte("Error")
		}
		return data

	// GET_VERSION returns the version of the node
	case protobufs.Request_GET_VERSION:
		return []byte("0.0 Hackney")

	}

	return []byte{}
}
