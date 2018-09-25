package networking

import (
	"github.com/dexm-coin/dexmd/wallet"
	bcp "github.com/dexm-coin/protobufs/build/blockchain"
	protoBlockchain "github.com/dexm-coin/protobufs/build/blockchain"
	protoNetwork "github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
	"gopkg.in/dedis/kyber.v2"
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
		cs.shardChain.AddMempoolTransaction(broadcastEnvelope.GetData())

	// Save a block proposed by a validator
	case protoNetwork.Broadcast_BLOCK_PROPOSAL:
		log.Printf("New Block: %x", broadcastEnvelope.GetData())

		block := &bcp.Block{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), block)
		if err != nil {
			log.Error("error on Unmarshal")
			return err
		}

		// save only the block that have cs.shardChain.currentblock+1
		// if block.Index != cs.shardChain.CurrentBlock+1 {
		// 	log.Error("The index of the block is wrong")
		// }
		// check if the signature of the block that should be cs.shardChain.CurrentValidator
		// if block.Miner != cs.shardChain.CurrentValidator {
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
		// verifyBlock, err := wallet.SignatureValid([]byte(cs.shardChain.CurrentValidator), r.Bytes(), s.Bytes(), hash)
		// if !verifyBlock || err != nil {
		// 	log.Error("SignatureValid ", err)
		// 	return err
		// }

		err = cs.shardChain.SaveBlock(block)
		if err != nil {
			log.Error("error on saving block")
			return err
		}
		err = cs.ImportBlock(block)
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
		if cs.beaconChain.Validators.CheckIsValidator(vote.PublicKey) {
			err := cs.AddVote(vote)
			if err != nil {
				log.Error(err)
				return err
			}
			cs.shardChain.CurrentVote++
		}

	case protoNetwork.Broadcast_WITHDRAW:
		log.Printf("New Withdraw: %x", broadcastEnvelope.GetData())

		withdrawVal := &protoBlockchain.ValidatorWithdraw{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), withdrawVal)
		if err != nil {
			log.Error(err)
			return err
		}

		cs.beaconChain.Validators.WithdrawValidator(withdrawVal.GetPublicKey(), withdrawVal.GetR(), withdrawVal.GetS(), int64(cs.shardChain.CurrentBlock))

	case protoNetwork.Broadcast_MERKLE_ROOTS:
		log.Printf("New Merkle Roots: %x", broadcastEnvelope.GetData())

		mr := &protoBlockchain.MerkleRoot{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), mr)
		if err != nil {
			log.Error(err)
			return err
		}

		// TODO verify the merkle root and save on MerkleRootsDb

		// func VerifySignature(message string, rSignature kyber.Point, sSignature kyber.Scalar, otherP []kyber.Point, myP kyber.Point, otherR []kyber.Point, myR kyber.Point) bool {

		transactions := mr.GetMerkleRootsTransaction()
		receipts := mr.GetMerkleRootsReceipt()
		validators := mr.GetValidators()
		// TODO change 2
		for i := 0; i < 2; i++ {
			// rValidators contain all the Rs except for 1, that is currentR
			// also get the public key of R
			var currentR kyber.Point
			var rValidators []kyber.Point
			var pValidators []kyber.Point
			for j, valByte := range mr.GetRValidators() {
				r, err := wallet.ByteToPoint(valByte)
				if err != nil {
					log.Error(err)
				}

				if j == i {
					currentR = r
					continue
				}
				rValidators = append(rValidators, r)

				p, err := cs.beaconChain.Validators.GetSchnorrPublicKey(validators[j])
				if err != nil {
					log.Error(err)
				}
				pValidators = append(pValidators, p)
			}

			currentP, err := cs.beaconChain.Validators.GetSchnorrPublicKey(validators[i])
			if err != nil {
				log.Error(err)
				return err
			}

			rSignedTransaction, err := wallet.ByteToPoint(mr.GetRSignedMerkleRootsTransaction())
			if err != nil {
				log.Error(err)
			}
			sSignedTransaction, err := wallet.ByteToScalar(mr.GetSSignedMerkleRootsTransaction())
			if err != nil {
				log.Error(err)
			}
			verify := wallet.VerifySignature(string(transactions[i]), rSignedTransaction, sSignedTransaction, pValidators, currentP, rValidators, currentR)
			if !verify {
				log.Error("Not verify")
			}

			rSignedReceipt, err := wallet.ByteToPoint(mr.GetRSignedMerkleRootsReceipt())
			if err != nil {
				log.Error(err)
			}
			sSignedReceipt, err := wallet.ByteToScalar(mr.GetSSignedMerkleRootsReceipt())
			if err != nil {
				log.Error(err)
			}
			verify = wallet.VerifySignature(string(receipts[i]), rSignedReceipt, sSignedReceipt, pValidators, currentP, rValidators, currentR)
			if !verify {
				log.Error("Not verify")
			}
		}

	case protoNetwork.Broadcast_SCHNORR:
		log.Printf("New Schnorr: %x", broadcastEnvelope.GetData())

		schnorr := &protoBlockchain.Schnorr{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), schnorr)
		if err != nil {
			log.Error(err)
			return err
		}
		cs.shardChain.Schnorr[schnorr.P] = schnorr.R
	}
	return nil
}
