package networking

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"reflect"
	"strconv"

	"github.com/dexm-coin/dexmd/util"
	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	"github.com/onrik/gomerkle"
	log "github.com/sirupsen/logrus"
)

// TODO put ReceiptBurned in a file, can't handle this in ram
var ReceiptBurned = make(map[string]bool)

func (cs *ConnectionStore) CheckMerkleProof(merkleProof *protobufs.MerkleProof) (bool, error) {
	// check if the proof has already be done
	if val, ok := ReceiptBurned[string(merkleProof.GetLeaf())]; ok && val {
		log.Error("Double spend!")
		return false, nil
	}
	shardMessage := merkleProof.GetShard()

	log.Info("VerifyProof in shard ", shardMessage)

	// if not, check the poof and then change the balance of the receiver and sender of the transaction in the proof
	if cs.VerifyProof(merkleProof, shardMessage) {
		// mark the hash of the transaction as burned
		ReceiptBurned[string(merkleProof.GetLeaf())] = true

		t := merkleProof.GetTransaction()
		sender := wallet.BytesToAddress(t.GetSender(), t.GetShard())

		// TODO check signature
		// result, _ := proto.Marshal(t)
		// bhash := sha256.Sum256(result)
		// hash := bhash[:]
		// valid, err := wallet.SignatureValid(t.GetSender(), t.GetR(), t.GetS(), hash)
		// if !valid || err != nil {
		// 	log.Error("SignatureValid ", err)
		// 	return false, err
		// }

		for interest := range cs.interests {
			shardInt, err := strconv.Atoi(interest)
			if err != nil {
				log.Error(err)
				continue
			}
			shard := uint32(shardInt)

			senderBalance, err := cs.shardsChain[shard].GetWalletState(sender)
			if err != nil {
				log.Error(err)
				return false, err
			}
			// Check if balance is sufficient
			requiredBal, ok := util.AddU64O(t.GetAmount(), uint64(t.GetGas()))
			if requiredBal > senderBalance.GetBalance() && ok {
				return false, errors.New("Balance is insufficient in transaction")
			}

			receiver := t.GetRecipient()
			// Ignore error because if the wallet doesn't exist yet we don't care
			reciverBalance, _ := cs.shardsChain[shard].GetWalletState(receiver)

			senderBalance.Balance -= t.GetAmount() + uint64(t.GetGas())
			reciverBalance.Balance += t.GetAmount()

			err = cs.shardsChain[shard].SetState(sender, &senderBalance)
			if err != nil {
				log.Error(err)
				return false, err
			}
			err = cs.shardsChain[shard].SetState(receiver, &reciverBalance)
			if err != nil {
				log.Error(err)
				return false, err
			}
		}
		return true, nil
	}
	return false, nil
}

// TODO maybe there is a problem with data and leaf

func hash(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}

func (cs *ConnectionStore) VerifyProof(mp *protobufs.MerkleProof, shard uint32) bool {
	//check that merkleproof.Root is inside MerkleRootsDb
	rootTransaction := mp.GetRoot()
	merkleRootFound := false
	if _, ok := cs.beaconChain.MerkleRootsDb[shard]; !ok {
		log.Error("if _, ok := cs.beaconChain.MerkleRootsDb[shard]; !ok {")
		return false
	}
	iter := cs.beaconChain.MerkleRootsDb[shard].NewIterator(nil, nil)
	for iter.Next() {
		value := iter.Value()

		mrs := &protobufs.MerkleRootsSigned{}
		proto.Unmarshal(value, mrs)
		for _, t := range mrs.GetMerkleRootsReceipt() {
			equal := reflect.DeepEqual(rootTransaction, t)
			if equal {
				merkleRootFound = true
				break
			}
		}
		if merkleRootFound {
			break
		}
	}
	iter.Release()
	err := iter.Error()
	if err != nil {
		log.Error(err)
	}

	if !merkleRootFound {
		log.Error("if !merkleRootFound {")
		return false
	}

	hashes := mp.GetMapHash()
	var mapProof []map[string][]byte
	for i, key := range mp.GetMapLeaf() {
		m := make(map[string][]byte)
		m[key] = hashes[i]
		mapProof = append(mapProof, m)
	}
	t, _ := proto.Marshal(mp.GetReceipt())

	equal := reflect.DeepEqual(hash(t), mp.GetLeaf())
	// check if the transaction and Leaf ( hash of the transaction for the proof ) are equal
	if !equal {
		log.Error("if !equal {")
		return false
	}

	return verifyMerkleProof(mapProof, mp.GetRoot(), mp.GetLeaf())
}

// VerifyProof verify proof for value
func verifyMerkleProof(proof []map[string][]byte, root, value []byte) bool {
	proofHash := value
	for _, p := range proof {
		if sibling, exist := p["left"]; exist {
			proofHash = hash(append(sibling, proofHash...))
		} else if sibling, exist := p["right"]; exist {
			proofHash = hash(append(proofHash, sibling...))
		} else {
			log.Error("verifyMerkleProof false")
			return false
		}
	}
	return bytes.Equal(root, proofHash)
}

func GenerateMerkleProof(receipts []*protobufs.Receipt, indexProof int, transaction *protobufs.Transaction, shard uint32) []byte {
	var data [][]byte
	for _, t := range receipts {
		tByte, _ := proto.Marshal(t)
		data = append(data, tByte)
	}

	tree := gomerkle.NewTree(sha256.New())
	tree.AddData(data...)

	err := tree.Generate()
	if err != nil {
		log.Error(err)
		return []byte{}
	}
	merkleRoot := tree.Root()

	proof := tree.GetProof(indexProof)
	leaf := tree.GetLeaf(indexProof)

	var listLeaf []string
	var listHash [][]byte
	for _, p := range proof {
		for key, value := range p {
			listLeaf = append(listLeaf, key)
			listHash = append(listHash, value)
		}
	}

	merkleProof := &protobufs.MerkleProof{
		MapLeaf:     listLeaf,
		MapHash:     listHash,
		Root:        merkleRoot,
		Leaf:        leaf,
		Receipt:     receipts[indexProof],
		Transaction: transaction,
		Shard:       shard,
	}
	merkleProofByte, _ := proto.Marshal(merkleProof)
	return merkleProofByte
}
