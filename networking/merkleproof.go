package networking

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"reflect"

	"github.com/dexm-coin/dexmd/util"
	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	"github.com/onrik/gomerkle"
	log "github.com/sirupsen/logrus"
)

var ReceiptBurned = make(map[string]bool)

func (cs *ConnectionStore) CheckMerkleProof(merkleProof *protobufs.MerkleProof, shard uint32) (bool, error) {
	// check if the proof has already be done
	if val, ok := ReceiptBurned[string(merkleProof.GetLeaf())]; ok && val {
		log.Error("Double spend!")
		return false, nil
	}

	// if not, check the poof and then change the balance of the receiver and sender of the transaction in the proof
	if cs.VerifyProof(merkleProof, shard) {
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

// TODO maybe there is a problem with data and leaf

func hash(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}

/*
panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x1 addr=0x178 pc=0x690071]

goroutine 218 [running]:
github.com/syndtr/goleveldb/leveldb.(*DB).isClosed(...)
        /home/antoniogroza/go/src/github.com/syndtr/goleveldb/leveldb/db_state.go:230
github.com/syndtr/goleveldb/leveldb.(*DB).ok(...)
        /home/antoniogroza/go/src/github.com/syndtr/goleveldb/leveldb/db_state.go:235
github.com/syndtr/goleveldb/leveldb.(*DB).NewIterator(0x0, 0x0, 0x0, 0x0, 0x0)
        /home/antoniogroza/go/src/github.com/syndtr/goleveldb/leveldb/db.go:879 +0x31
github.com/dexm-coin/dexmd/networking.(*ConnectionStore).VerifyProof(0xc4200aa0c0, 0xc423f70000, 0xc400000000, 0x20)
        /home/antoniogroza/go/src/github.com/dexm-coin/dexmd/networking/merkleproof.go:94 +0xac
github.com/dexm-coin/dexmd/networking.(*ConnectionStore).CheckMerkleProof(0xc4200aa0c0, 0xc423f70000, 0x0, 0x9c5aa0, 0xc423f70000, 0x0)
        /home/antoniogroza/go/src/github.com/dexm-coin/dexmd/networking/merkleproof.go:27 +0xb3
github.com/dexm-coin/dexmd/networking.(*ConnectionStore).handleBroadcast(0xc4200aa0c0, 0xc425206000, 0x184, 0x1a0, 0x0, 0x2, 0x982700)
        /home/antoniogroza/go/src/github.com/dexm-coin/dexmd/networking/broadcast.go:244 +0x1cec
created by github.com/dexm-coin/dexmd/networking.(*client).read
		/home/antoniogroza/go/src/github.com/dexm-coin/dexmd/networking/server.go:353 +0x285
*/

func (cs *ConnectionStore) VerifyProof(mp *protobufs.MerkleProof, shard uint32) bool {
	//check that merkleproof.Root is inside MerkleRootsDb
	rootTransaction := mp.GetRoot()
	merkleRootFound := false
	log.Info("VerifyProof")
	log.Info("cs.beaconChain.MerkleRootsDb ", cs.beaconChain.MerkleRootsDb)
	log.Info("cs.beaconChain.MerkleRootsDb[shard] ", cs.beaconChain.MerkleRootsDb[shard])
	if _, ok := cs.beaconChain.MerkleRootsDb[shard]; !ok {
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
			return false
		}
	}
	return bytes.Equal(root, proofHash)
}

func GenerateMerkleProof(receipts []*protobufs.Receipt, indexProof int, transaction *protobufs.Transaction) []byte {
	var data [][]byte
	for _, t := range receipts {
		tByte, _ := proto.Marshal(t)
		data = append(data, tByte)
	}

	tree := gomerkle.NewTree(sha256.New())
	tree.AddData(data...)

	err := tree.Generate()
	if err != nil {
		panic(err)
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
	}
	merkleProofByte, _ := proto.Marshal(merkleProof)
	return merkleProofByte
}
