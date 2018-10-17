package networking

import (
	"errors"
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

func (cs *ConnectionStore) handleBroadcast(data []byte, shard uint32) error {
	broadcastEnvelope := &protoNetwork.Broadcast{}
	err := proto.Unmarshal(data, broadcastEnvelope)
	if err != nil {
		return err
	}

	// log.Info("Broadcast type:", broadcastEnvelope.GetType())

	switch broadcastEnvelope.GetType() {
	// Register a new transaction to the mempool
	case protoNetwork.Broadcast_TRANSACTION:
		pb := &protoBlockchain.Transaction{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), pb)
		if err != nil {
			log.Error(err)
			return err
		}

		// check the interests with the shard of the sender of the transaction
		if !cs.CheckShard(pb.GetShard()) {
			return nil
		}

		log.Printf("New Transaction: %x", broadcastEnvelope.GetData())
		cs.shardsChain[shard].AddMempoolTransaction(pb, broadcastEnvelope.GetData())

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

		// // save only the block that have cs.shardsChain[shard].currentblock+1
		// if block.Index != cs.shardsChain[shard].CurrentBlock+1 {
		// 	log.Error("The index of the block is wrong")
		// }

		// TODO the message arrive too fast so CurrentValidator didn't get update to check if it is right
		// check if the miner of the block that should be cs.shardsChain[shard].CurrentValidator[block.GetIndex()]
		// if block.Miner != cs.shardsChain[shard].CurrentValidator[block.GetIndex()] {
		// 	log.Error("The miner is wrong")
		// 	return err
		// }

		// TODO check signature
		// blockBytes, _ := proto.Marshal(block)
		// bhash := sha256.Sum256(blockBytes)
		// hash := bhash[:]
		// r, s, err := cs.identity.Sign(hash)
		// if err != nil {
		// 	log.Error(err)
		// 	return err
		// }

		// TODO change first parameter
		// verifyBlock, err := wallet.SignatureValid([]byte(cs.shardsChain[shard].CurrentValidator), r.Bytes(), s.Bytes(), hash)
		// if !verifyBlock || err != nil {
		// 	log.Error("SignatureValid ", err)
		// 	return err
		// }

		err = cs.SaveBlock(block, shard)
		if err != nil {
			log.Error("error on saving block")
			return err
		}
		err = cs.ImportBlock(block, uint32(shard))
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
			err := cs.AddVote(vote, shard)
			if err != nil {
				log.Error(err)
				return err
			}
			cs.shardsChain[shard].CurrentVote++
		}

	case protoNetwork.Broadcast_WITHDRAW:
		log.Printf("New Withdraw: %x", broadcastEnvelope.GetData())

		withdrawVal := &protoBlockchain.ValidatorWithdraw{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), withdrawVal)
		if err != nil {
			log.Error(err)
			return err
		}

		cs.beaconChain.Validators.WithdrawValidator(withdrawVal.GetPublicKey(), withdrawVal.GetR(), withdrawVal.GetS(), int64(cs.shardsChain[shard].CurrentBlock))

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
		cs.shardsChain[shard].Schnorr[schnorr.P] = schnorr.R

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

		// TODO before add it, i have to check if the signature is valid
		// also it is possible that the signature is right, but the message inside is fake

		cs.shardsChain[shard].MTReceipt = append(cs.shardsChain[shard].MTReceipt, signSchnorr.GetSignReceipt())
		cs.shardsChain[shard].RSchnorr = append(cs.shardsChain[shard].RSchnorr, signSchnorr.GetRSchnorr())
		cs.shardsChain[shard].PSchnorr = append(cs.shardsChain[shard].PSchnorr, signSchnorr.GetPSchnorr())
		cs.shardsChain[shard].MessagesReceipt = append(cs.shardsChain[shard].MessagesReceipt, signSchnorr.GetMessageSignReceipt())

	case protoNetwork.Broadcast_MERKLE_ROOTS_SIGNED:
		log.Printf("New Merkle Roots: %x", broadcastEnvelope.GetData())

		mr := &protoBlockchain.MerkleRootsSigned{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), mr)
		if err != nil {
			log.Error(err)
			return err
		}

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

		counterSignatureRight := 0
		// over all signed merkle roots check if they are verified
		for i := 0; i < len(receipts); i++ {
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
			verify := wallet.VerifySignature(string(receipts[i]), rSignedReceipt, sSignedReceipt, pValidators, rValidators)
			if !verify {
				log.Error("Not verify")
				return errors.New("Verify failed")
			}
			counterSignatureRight++
		}

		// check 2/3 validate firm the merkleroot
		if float64(cs.beaconChain.Validators.LenValidators(mr.GetShard())) < float64(2*counterSignatureRight/3) {
			log.Error("2/3 Didn't validate")
			return nil
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

	case protoNetwork.Broadcast_MONEY_WITHDRAW:
		log.Printf("New Money WithDraw: %x", broadcastEnvelope.GetData())

		moneyWithdraw := &protoBlockchain.MoneyWithdraw{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), moneyWithdraw)
		if err != nil {
			log.Error(err)
			return err
		}

		if !cs.beaconChain.Validators.CheckWithdraw(moneyWithdraw.GetWallet(), cs.shardsChain[shard]) {
			log.Error("CheckWithdraw failed")
		} else {
			log.Info("CheckWithdraw correct")
		}
	}
	return nil
}
