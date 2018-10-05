package networking

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strconv"

	"github.com/dexm-coin/dexmd/wallet"
	bcp "github.com/dexm-coin/protobufs/build/blockchain"
	protoBlockchain "github.com/dexm-coin/protobufs/build/blockchain"
	protoNetwork "github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
	kyber "gopkg.in/dedis/kyber.v2"
)

// CheckShard check if the message arrived is from your interests (shards)
func (cs *ConnectionStore) CheckShard(shard uint32) bool {
	if shard != 0 {
		for interest := range cs.interests {
			interestInt, err := strconv.Atoi(interest)
			if err != nil {
				log.Error(err)
				continue
			}
			if shard == uint32(interestInt) {
				return true
			}
		}
		return false
	}
	return true
}

func (cs *ConnectionStore) handleBroadcast(data []byte, shard uint32) error {
	broadcastEnvelope := &protoNetwork.Broadcast{}
	err := proto.Unmarshal(data, broadcastEnvelope)
	if err != nil {
		return err
	}

	// check if the interest exist in interestedClients
	if _, ok := cs.interestedClients[fmt.Sprint(shard)]; ok {
		// if so, send the message to the interest client
		y := math.Exp(float64(20/len(cs.interestedClients[fmt.Sprint(shard)]))) - 0.5
		for k := range cs.interestedClients[fmt.Sprint(shard)] {

			broadcastEnvelope.TTL--
			if broadcastEnvelope.TTL < 1 || broadcastEnvelope.TTL > 64 {
				continue
			}

			broadcastBytes, err := proto.Marshal(broadcastEnvelope)
			if err != nil {
				log.Error(err)
				continue
			}
			newEnv := &protoNetwork.Envelope{
				Type:  protoNetwork.Envelope_BROADCAST,
				Data:  broadcastBytes,
				Shard: shard,
			}
			dataByte, err := proto.Marshal(newEnv)
			if err != nil {
				log.Error(err)
				continue
			}

			if rand.Float64() > y {
				continue
			}
			if k.isOpen {
				k.send <- dataByte
			}
		}
	}

	log.Info("Broadcast type:", broadcastEnvelope.GetType())

	switch broadcastEnvelope.GetType() {
	// Register a new transaction to the mempool
	case protoNetwork.Broadcast_TRANSACTION:
		if !cs.CheckShard(shard) {
			return nil
		}

		log.Printf("New Transaction: %x", broadcastEnvelope.GetData())
		cs.shardChain.AddMempoolTransaction(broadcastEnvelope.GetData())

	// Save a block proposed by a validator
	case protoNetwork.Broadcast_BLOCK_PROPOSAL:
		if !cs.CheckShard(shard) {
			return nil
		}

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

		log.Info("Save block ", block.Index)

	case protoNetwork.Broadcast_CHECKPOINT_VOTE:
		if !cs.CheckShard(shard) {
			return nil
		}

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

	case protoNetwork.Broadcast_SCHNORR:
		if !cs.CheckShard(shard) {
			return nil
		}

		log.Printf("New Schnorr: %x", broadcastEnvelope.GetData())

		schnorr := &protoBlockchain.Schnorr{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), schnorr)
		if err != nil {
			log.Error(err)
			return err
		}
		cs.shardChain.Schnorr[schnorr.P] = schnorr.R

	case protoNetwork.Broadcast_SIGN_SCHNORR:
		if !cs.CheckShard(shard) {
			return nil
		}

		log.Printf("New Sign Schnorr: %x", broadcastEnvelope.GetData())

		signSchnorr := &protoBlockchain.SignSchnorr{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), signSchnorr)
		if err != nil {
			log.Error(err)
			return err
		}

		cs.shardChain.MTTrasaction = append(cs.shardChain.MTTrasaction, signSchnorr.GetSignTransaction())
		cs.shardChain.MTReceipt = append(cs.shardChain.MTReceipt, signSchnorr.GetSignReceipt())
		cs.shardChain.RSchnorr = append(cs.shardChain.RSchnorr, signSchnorr.GetRSchnorr())
		cs.shardChain.PSchnorr = append(cs.shardChain.PSchnorr, signSchnorr.GetPSchnorr())
		cs.shardChain.MessagesTransaction = append(cs.shardChain.MessagesTransaction, signSchnorr.GetMessageSignTransaction())
		cs.shardChain.MessagesReceipt = append(cs.shardChain.MessagesReceipt, signSchnorr.GetMessageSignReceipt())

	case protoNetwork.Broadcast_MERKLE_ROOTS_SIGNED:
		log.Printf("New Merkle Roots: %x", broadcastEnvelope.GetData())

		mr := &protoBlockchain.MerkleRootsSigned{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), mr)
		if err != nil {
			log.Error(err)
			return err
		}

		transactions := mr.GetMerkleRootsTransaction()
		receipts := mr.GetMerkleRootsReceipt()
		pValidatorsByte := mr.GetPValidators()

		// get all P and R from the validator that signed the merkle roots
		var rValidators []kyber.Point
		var pValidators []kyber.Point
		for j, valByte := range mr.GetRValidators() {
			r, err := wallet.ByteToPoint(valByte)
			if err != nil {
				log.Error(err)
				return err
			}
			rValidators = append(rValidators, r)

			p, err := wallet.ByteToPoint(pValidatorsByte[j])
			if err != nil {
				log.Error(err)
				return err
			}
			pValidators = append(pValidators, p)
		}

		// TODO check 2/3 validator signed correctly

		// over all signed merkle roots check if they are verified
		for i := 0; i < len(transactions); i++ {
			rSignedTransaction, err := wallet.ByteToPoint(mr.GetRSignedMerkleRootsTransaction())
			if err != nil {
				log.Error(err)
				return err
			}
			sSignedTransaction, err := wallet.ByteToScalar(mr.GetSSignedMerkleRootsTransaction())
			if err != nil {
				log.Error(err)
				return err
			}
			verify := wallet.VerifySignature(string(transactions[i]), rSignedTransaction, sSignedTransaction, pValidators, rValidators)
			if !verify {
				log.Error("Not verify")
				return errors.New("Verify failed")
			}

			rSignedReceipt, err := wallet.ByteToPoint(mr.GetRSignedMerkleRootsReceipt())
			if err != nil {
				log.Error(err)
				return err
			}
			sSignedReceipt, err := wallet.ByteToScalar(mr.GetSSignedMerkleRootsReceipt())
			if err != nil {
				log.Error(err)
				return err
			}
			verify = wallet.VerifySignature(string(receipts[i]), rSignedReceipt, sSignedReceipt, pValidators, rValidators)
			if !verify {
				log.Error("Not verify")
				return errors.New("Verify failed")
			}
		}

		// TODO right now i save every Broadcast_MERKLE_ROOTS_SIGNED that is verified, but i can't do like this

		// if everything is verified then save the signature inside the MerkleRootsDb
		err = cs.beaconChain.SaveMerkleRoots(mr)
		if err != nil {
			log.Error(err)
			return err
		}
		log.Info("MerkleRootSigned Verified")

	case protoNetwork.Broadcast_MERKLE_PROOF:
		log.Printf("New Merkle Proof: %x", broadcastEnvelope.GetData())

		merkleProof := &protoBlockchain.MerkleProof{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), merkleProof)
		if err != nil {
			log.Error(err)
			return err
		}
 
		ok, err := cs.CheckMerkleProof(merkleProof, shard)
		if err != nil || !ok {
			log.Error("Proof not verifed", err)
			return err
		}
		log.Info("Proof verifed")

	}
	return nil
}
