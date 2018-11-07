package networking

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strconv"
	"time"

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
	if shard == 0 {
		return false
	}
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

func (cs *ConnectionStore) handleBroadcast(data []byte, shard uint32, identity *protoNetwork.Signature) error {
	broadcastEnvelope := &protoNetwork.Broadcast{}
	err := proto.Unmarshal(data, broadcastEnvelope)
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(broadcastEnvelope.GetAddress(), identity.GetPubkey()) {
		log.Error("broadcast.GetAddress() != identity.GetPubkey()")
		return err
	}

	// log.Info("Broadcast type:", broadcastEnvelope.GetType())

	switch broadcastEnvelope.GetType() {
	// Register a new transaction to the mempool
	case protoNetwork.Broadcast_TRANSACTION:
		transaction := &protoBlockchain.Transaction{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), transaction)
		if err != nil {
			log.Error(err)
			return err
		}

		// check the interests with the shard of the sender of the transaction
		if !cs.CheckShard(transaction.GetShard()) {
			return nil
		}

		log.Printf("New Transaction: %x", broadcastEnvelope.GetData())
		cs.shardsChain[transaction.GetShard()].AddMempoolTransaction(transaction, broadcastEnvelope.GetData())

	// Save a block proposed by a validator
	case protoNetwork.Broadcast_BLOCK_PROPOSAL:
		if !cs.CheckShard(shard) {
			return nil
		}
		byteBlock := broadcastEnvelope.GetData()

		log.Printf("New Block: %x", byteBlock)

		block := &bcp.Block{}
		err := proto.Unmarshal(byteBlock, block)
		if err != nil {
			log.Error("error on Unmarshal")
			return err
		}

		log.Info("New Block from ", block.GetMiner())

		// check if the miner of the block that should be cs.shardsChain[shard].CurrentValidator[block.GetIndex()]
		if block.GetMiner() != cs.shardsChain[shard].CurrentValidator[block.GetIndex()] {
			log.Error("The miner is wrong")
			log.Error("Should be ", cs.shardsChain[shard].CurrentValidator[block.GetIndex()])
			return err
		}

		bc := cs.shardsChain[shard]

		prevHash := block.GetPrevHash()
		h := sha256.New()
		h.Write(prevHash)
		hashPrevBlock := hex.EncodeToString(h.Sum(nil))
		h2 := sha256.New()
		h2.Write(byteBlock)
		hashCurrentBlock := hex.EncodeToString(h.Sum(nil))

		// calulate the priotity, based on when is arrived
		arrivalOrder := float64(500-(time.Now().UnixNano()/int64(time.Millisecond))%500) * 0.001
		// check if this block is arrived after a block that point to this block
		if missingCurrentBlockValue, ok := bc.MissingBlock[hashCurrentBlock]; ok {
			// if the prevhash of this block has already arrived
			// if you have [1, 2, ?, 4] and now is arrived 3
			if _, ok2 := bc.HashBlocks[hashPrevBlock]; ok2 {
				//  recalculate the height of every block that was inside blocksToRecalculate
				lenBlocks := len(missingCurrentBlockValue.GetMBbBlocksToRecalculate())
				for index, blockToRecalulate := range missingCurrentBlockValue.GetMBbBlocksToRecalculate() {
					bc.HashBlocks[blockToRecalulate] = bc.HashBlocks[hashPrevBlock] + uint64(lenBlocks-index)
				}
				// calulate the height of the hightest block and insert it on HashBlocks
				hightestBlock := missingCurrentBlockValue.GetMBHightestBlock()
				bc.HashBlocks[hightestBlock] = missingCurrentBlockValue.GetMBSequenceBlock() + bc.HashBlocks[hashPrevBlock]
				// remove hashCurrentBlock from MissingBlock, and add the hightest block to the queue
				// key = hash highest block (in this case 4), value = height of the hightest block + the arrival order (that is (500-ts%500)*0.001)
				finalPriotityBlock := float64(bc.HashBlocks[hightestBlock]) + missingCurrentBlockValue.GetMBArrivalOrder()
				bc.PriorityBlocks.Insert(missingCurrentBlockValue.GetMBHightestBlock(), finalPriotityBlock)
				delete(bc.MissingBlock, hashCurrentBlock)
			} else {
				// if hasn't arrived remove the previous block (so this one) and add this new one, but adding 1 to the sequence
				// if you have [1, ?, ?, 4] and now is arrived 3
				bc.ModifyMissingBlock(hashPrevBlock, missingCurrentBlockValue, hashCurrentBlock)
				delete(bc.MissingBlock, hashCurrentBlock)
			}
		} else {
			// if you have [1, 2, ?] and now is arrived 3
			if _, ok := bc.HashBlocks[hashPrevBlock]; ok {
				// calulate the priotity and add it to PriorityBlocks
				bc.PriorityBlocks.Insert(hashCurrentBlock, float64(bc.HashBlocks[hashPrevBlock])+1+arrivalOrder)
				// also add it to the block added with height prev block + 1
				bc.HashBlocks[hashCurrentBlock] = bc.HashBlocks[hashPrevBlock] + 1
			} else {
				// if you have [1, ?, ?] and now is arrived 3
				// add it to the missing blocks
				bc.AddMissingBlock(hashPrevBlock, arrivalOrder, hashCurrentBlock)
			}
		}

		// // save only the block that have cs.shardsChain[shard].currentblock
		// if block.Index != cs.shardsChain[shard].CurrentBlock {
		// 	log.Error("The index of the block is wrong")
		// 	return err
		// }

		// blockValid := true
		// for i := bc.CurrentBlock - 1; i >= 0; i-- {
		// 	currBlock, err := bc.GetBlock(i)
		// 	if err != nil {
		// 		continue
		// 	}
		// 	bhash := sha256.Sum256(currBlock)
		// 	hash := bhash[:]
		// 	equal := reflect.DeepEqual(hash, block.GetPrevHash())
		// 	if !equal {
		// 		// TODO ASAP ask to the network if my prevhash (of currentblock) exist
		// 		log.Error("the prev hash doen't match with the block")
		// 		cs.RequestHashBlock(shard, i, hash)
		// 		if verify {
		// 			// the network agree with me that this block doesn't exist, so is a fake
		// 			blockValid = false
		// 		} else {
		// 			// replace the block in this index with the new one
		// 		}
		// 	} else {
		// 		if i != bc.CurrentBlock-1 {
		// 			// TODO ASAP remove the previous blocks if they exists
		// 		}
		// 	}
		// }
		// if !blockValid {
		// 	return err
		// }

		_, err = bc.GetBlock(block.GetIndex())
		if err == nil {
			log.Info("slash for ", block.GetMiner())
			err = cs.beaconChain.Validators.RemoveValidator(block.GetMiner())
			if err != nil {
				log.Error("error on RemoveValidator")
			}
			return err
		}

		err = cs.SaveBlock(block, shard)
		if err != nil {
			log.Error("error on saving block")
			return err
		}
		// err = cs.ImportBlock(block, uint32(shard))
		// if err != nil {
		// 	log.Error("error on importing block")
		// 	return err
		// }

		log.Info("Save block ", block.Index)

	// case protoNetwork.Broadcast_CHECKPOINT_VOTE:
	// 	if !cs.CheckShard(shard) {
	// 		return nil
	// 	}

	// 	log.Printf("New CasperVote: %x", broadcastEnvelope.GetData())

	// 	vote := &protoBlockchain.CasperVote{}
	// 	err := proto.Unmarshal(broadcastEnvelope.GetData(), vote)
	// 	if err != nil {
	// 		log.Error(err)
	// 		return err
	// 	}
	// 	if cs.beaconChain.Validators.CheckIsValidator(vote.PublicKey) {
	// 		err := cs.AddVote(vote, shard)
	// 		if err != nil {
	// 			log.Error(err)
	// 			return err
	// 		}
	// 		cs.shardsChain[shard].CurrentVote++
	// 	}

	case protoNetwork.Broadcast_WITHDRAW:
		log.Printf("New Withdraw: %x", broadcastEnvelope.GetData())

		withdrawVal := &protoBlockchain.ValidatorWithdraw{}
		err := proto.Unmarshal(broadcastEnvelope.GetData(), withdrawVal)
		if err != nil {
			log.Error(err)
			return err
		}

		sender := wallet.BytesToAddress(withdrawVal.GetPublicKey(), 1)
		cs.beaconChain.Validators.WithdrawValidator(sender, int64(cs.shardsChain[shard].CurrentBlock))

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

		// TODO this is wrong maybe, i have to check the shard
		if !cs.CheckShard(merkleProof.GetShard()) {
			log.Error("Not your shard")
			return err
		}

		ok, err := cs.CheckMerkleProof(merkleProof)
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
