package networking

import (
	"fmt"
	"strconv"

	protobufs "github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
)

func (cs *ConnectionStore) handleMessage(pb *protobufs.Request, c *client, shard uint32) []byte {
	dataMessage := pb.GetData()

	switch pb.GetType() {
	// GET_BLOCKCHAIN_LEN returns the current block index
	case protobufs.Request_GET_BLOCKCHAIN_LEN:
		return []byte(strconv.FormatUint(cs.shardsChain[shard].CurrentBlock, 10))

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

	// GET_BLOCK returns a block at the passed index and shard
	case protobufs.Request_GET_BLOCK:
		if !cs.CheckShard(shard) {
			return []byte("Error")
		}

		if len(dataMessage.GetData()) != 1 {
			return []byte("Error")
		}

		index := uint64(dataMessage.GetData()[0])

		block, err := cs.shardsChain[shard].GetBlock(index)
		if err != nil {
			return []byte("Error")
		}
		return block

	// GET_WALLET_STATUS returns the current balance and nonce of a wallet
	case protobufs.Request_GET_WALLET_STATUS:
		if !cs.CheckShard(shard) {
			return []byte("Error")
		}

		state, err := cs.shardsChain[shard].GetWalletState(fmt.Sprintf("%s", dataMessage.GetData()))
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

	// GET_CONTRACT_CODE returns the code of a contract
	case protobufs.Request_GET_CONTRACT_CODE:
		if !cs.CheckShard(shard) {
			return []byte("Error")
		}

		code, err := cs.shardsChain[shard].GetContractCode(dataMessage.GetData())
		if err != nil {
			return []byte("Error")
		}

		return code

	// GET_INTERESTS returns the type of broadcasts the client is interested in
	case protobufs.Request_GET_INTERESTS:
		keys := []string{}

		for k := range cs.interests {
			keys = append(keys, k)
		}

		p := &protobufs.Interests{
			Keys: keys,
		}
		log.Info("Request_GET_INTERESTS ", p)

		d, _ := proto.Marshal(p)
		return d
	}

	return []byte{}
}
