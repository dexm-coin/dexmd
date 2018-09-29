package networking

import (
	"errors"

	"github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/util"
	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
)

var ReceiptBurned = make(map[string]bool)

func (cs *ConnectionStore) CheckMerkleProof(merkleProof *protobufs.MerkleProof) (bool, error) {
	// check if the proof has already be done
	if val, ok := ReceiptBurned[string(merkleProof.GetLeaf())]; ok && val {
		log.Error("Double spend!")
		return false, nil
	}

	// if not, check the poof and then change the balance of the receiver and sender of the transaction in the proof
	if blockchain.VerifyProof(merkleProof) {
		// mark the hash of the transaction as burned
		ReceiptBurned[string(merkleProof.GetLeaf())] = true

		t := merkleProof.GetTransaction()
		result, _ := proto.Marshal(t)
		sender := wallet.BytesToAddress(t.GetSender(), t.GetShard())

		valid, err := wallet.SignatureValid(t.GetSender(), t.GetR(), t.GetS(), result)
		if !valid || err != nil {
			log.Error("SignatureValid ", err)
			return false, err
		}

		senderBalance, err := cs.shardChain.GetWalletState(sender)
		if err != nil {
			log.Error(err)
			return false, err
		}
		// Check if balance is sufficient
		requiredBal, ok := util.AddU64O(t.GetAmount(), uint64(t.GetGas()))
		if requiredBal > senderBalance.GetBalance() && ok {
			return false, errors.New("Balance is insufficient in transaction")
		}
		// Check if nonce is correct
		newNonce, ok := util.AddU32O(senderBalance.GetNonce(), uint32(1))
		if t.GetNonce() != newNonce || !ok {
			return false, errors.New("Invalid nonce in transaction")
		}

		receiver := t.GetRecipient()
		// Ignore error because if the wallet doesn't exist yet we don't care
		reciverBalance, _ := cs.shardChain.GetWalletState(receiver)

		senderBalance.Balance -= t.GetAmount() + uint64(t.GetGas())
		reciverBalance.Balance += t.GetAmount()

		senderBalance.Nonce++

		err = cs.shardChain.SetState(sender, &senderBalance)
		if err != nil {
			log.Error(err)
			return false, err
		}
		err = cs.shardChain.SetState(receiver, &reciverBalance)
		if err != nil {
			log.Error(err)
			return false, err
		}

		return true, nil
	}
	return false, nil
}
