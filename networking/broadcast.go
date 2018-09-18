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

	// Save a block proposed by a validator
	case protoNetwork.Broadcast_BLOCK_PROPOSAL:
		log.Printf("New Block: %x", broadcastEnvelope.GetData())

		block := &bcp.Block{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), block)
		if err != nil {
			log.Error("error on Unmarshal")
			return err
		}

		// save only the block that have cs.bc.currentblock+1
		// if block.Index != cs.bc.CurrentBlock+1 {
		// 	log.Error("The index of the block is wrong")
		// }
		// check if the signature of the block that should be cs.bc.CurrentValidator
		// if block.Miner != cs.bc.CurrentValidator {
		// 	log.Error("The miner is wrong")
		// }
		// blockBytes, _ := proto.Marshal(block)
		// bhash := sha256.Sum256(blockBytes)
		// hash := bhash[:]
		// r, s, err := cs.identity.Sign(hash)
		// if err != nil {
		// 	log.Error(err)
		// 	return err
		// }
		// verifyBlock, err := wallet.SignatureValid([]byte(cs.bc.CurrentValidator), r.Bytes(), s.Bytes(), hash)
		// if !verifyBlock || err != nil {
		// 	log.Error("SignatureValid ", err)
		// 	return err
		// }

		err = cs.bc.SaveBlock(block)
		if err != nil {
			log.Error("error on saving block")
			return err
		}
		err = cs.bc.ImportBlock(block)
		if err != nil {
			log.Error("error on importing block")
			return err
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

		withdrawVal := &protoBlockchain.ValidatorWithdraw{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), withdrawVal)
		if err != nil {
			log.Error(err)
			return err
		}

		cs.bc.Validators.WithdrawValidator(withdrawVal.GetPublicKey(), withdrawVal.GetR(), withdrawVal.GetS(), int64(cs.bc.CurrentBlock))
	}
	return nil
}
