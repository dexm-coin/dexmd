package networking

import (
	"strconv"

	"github.com/dexm-coin/dexmd/blockchain"
	protobufs "github.com/dexm-coin/protobufs/build/network"
)

func handleMessage(bc blockchain.Blockchain, pb *protobufs.Request) []byte {
	switch pb.GetType() {
	// GET_BLOCKCHAIN_LEN returns the current block index
	case protobufs.Request_GET_BLOCKCHAIN_LEN:
		return []byte(strconv.FormatUint(bc.CurrentBlock, 10))
	}

	return []byte{}
}
