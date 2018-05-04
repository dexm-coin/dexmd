package networking

import (
	"strconv"

	protobufs "github.com/dexm-coin/protobufs/build/network"
	"github.com/gogo/protobuf/proto"
)

func (cs *ConnectionStore) handleMessage(pb *protobufs.Request) []byte {
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
		block, err := cs.bc.GetBlocks(pb.GetIndex())
		if err != nil {
			return []byte("Error")
		}

		return block

	}

	return []byte{}
}
