package networking

import (
	bcp "github.com/dexm-coin/protobufs/build/blockchain"
	protobufs "github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
)

func handleBroadcast(data []byte) error {
	broadcastEnvelope := &protobufs.Broadcast{}
	err := proto.Unmarshal(data, broadcastEnvelope)
	if err != nil {
		return err
	}

	switch broadcastEnvelope.GetType() {
	// Register a new transaction to the mempool
	case protobufs.Broadcast_TRANSACTION:
		return bc.AddMempoolTransaction(broadcastEnvelope.GetData())

		// Save a block proposed by a validator TODO Check if it's his turn
	case protobufs.Broadcast_BLOCK_PROPOSAL:
		block := &bcp.Block{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), block)
		if err != nil {
			return err
		}

		return bc.SaveBlock(block)
	}

	return nil
}
