package blockchain

import (
	"bytes"
	"crypto/sha256"
	"reflect"

	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	"github.com/onrik/gomerkle"
)

// TODO maybe there is a problem with data and leaf

func hash(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}

func VerifyProof(mp *protobufs.MerkleProof) bool {
	// TODO check that merkleproof.Root is inside the merklerootDB
	hashes := mp.GetMapHash()
	var mapProof []map[string][]byte
	for i, key := range mp.GetMapLeaf() {
		m := make(map[string][]byte)
		m[key] = hashes[i]
		mapProof = append(mapProof, m)
	}
	t, _ := proto.Marshal(mp.GetTransaction())

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

func GenerateMerkleTree(transactions []*protobufs.Transaction) ([]byte, []byte, error) {
	var dataTransaction [][]byte
	var dataReceipt [][]byte
	for _, t := range transactions {
		r := &protobufs.Receipt{
			Sender:    wallet.BytesToAddress(t.GetSender(), t.GetShard()),
			Recipient: t.GetRecipient(),
			Amount:    t.GetAmount(),
			Nonce:     t.GetNonce(),
		}
		rByte, _ := proto.Marshal(r)
		tByte, _ := proto.Marshal(t)
		dataTransaction = append(dataTransaction, tByte)
		dataReceipt = append(dataReceipt, rByte)
	}

	treeTransaction := gomerkle.NewTree(sha256.New())
	treeTransaction.AddData(dataTransaction...)
	err := treeTransaction.Generate()
	if err != nil {
		return nil, nil, err
	}
	merkleRootTransaction := treeTransaction.Root()

	treeReceipt := gomerkle.NewTree(sha256.New())
	treeReceipt.AddData(dataReceipt...)
	err = treeReceipt.Generate()
	if err != nil {
		return nil, nil, err
	}
	merkleRootReceipt := treeReceipt.Root()

	return merkleRootTransaction, merkleRootReceipt, nil
}

func GenerateMerkleProof(transactions []*protobufs.Transaction, indexProof int) []byte {
	var data [][]byte
	for _, t := range transactions {
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
		Transaction: transactions[indexProof],
	}
	merkleProofByte, _ := proto.Marshal(merkleProof)
	return merkleProofByte
}
