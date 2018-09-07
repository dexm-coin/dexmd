package networking

import (
	bcp "github.com/dexm-coin/protobufs/build/blockchain"
	protoBlockchain "github.com/dexm-coin/protobufs/build/blockchain"
	protoNetwork "github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
)

func (cs *ConnectionStore) handleBroadcast(data []byte) error {
	broadcastEnvelope := &protoNetwork.Broadcast{}
	err := proto.Unmarshal(data, broadcastEnvelope)
	if err != nil {
		return err
	}

	log.Info("Broadcast type:", broadcastEnvelope.GetType())

	switch broadcastEnvelope.GetType() {
	// Register a new transaction to the mempool
	case protoNetwork.Broadcast_TRANSACTION:
		log.Printf("New Transaction: %x", broadcastEnvelope.GetData())
		cs.bc.AddMempoolTransaction(broadcastEnvelope.GetData())

	// Save a block proposed by a validator TODO Verify turn and identity
	case protoNetwork.Broadcast_BLOCK_PROPOSAL:
		log.Printf("New Block: %x", broadcastEnvelope.GetData())

		block := &bcp.Block{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), block)
		if err != nil {
			return err
		}

		// TODO check if the signature of the block that should be cs.bc.CurrentValidator
		// TODO check the timestamp of the block, if it's "wrong" don't accept it
		err = cs.bc.SaveBlock(block)
		if err != nil {
			log.Error("error on saving block")
		}
		err = cs.bc.ImportBlock(block)
		if err != nil {
			log.Error("error on importing block")
		}

	case protoNetwork.Broadcast_CHECKPOINT_VOTE:
		log.Printf("New CasperVote: %x", broadcastEnvelope.GetData())

		vote := &protoBlockchain.CasperVote{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), vote)
		if err != nil {
			log.Error(err)
			return err
		}
		if cs.bc.Validators.CheckIsValidator(vote.PublicKey) {
			err := cs.bc.AddVote(vote)
			if err != nil {
				log.Error(err)
				return err
			}
			cs.bc.CurrentVote++
		}

	case protoNetwork.Broadcast_WITHDRAW:
		log.Printf("New Withdraw: %x", broadcastEnvelope.GetData())

		withdrawValidator := &protoBlockchain.ValidatorWithdraw{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), withdrawValidator)
		if err != nil {
			log.Error(err)
			return err
		}

		cs.bc.Validators.WithdrawValidator(withdrawValidator.GetPublicKey(), int64(cs.bc.CurrentBlock))
	}
	return nil
}
