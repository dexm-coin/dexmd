package tests

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"

	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	"github.com/onrik/gomerkle"
	// "github.com/golang/protobuf/proto"
)

// func TestGenerationMerkleTree(t *testing.T) {
// 	// Generate a fake genesis block for testing
// 	w1, _ := wallet.GenerateWallet()
// 	w2, _ := wallet.GenerateWallet()

// 	recipient, _ := w2.GetWallet()

// 	w1.Balance = 10000
// 	transaction1, _ := w1.NewTransaction(recipient, 100, 1, []byte{})
// 	transaction2, _ := w1.NewTransaction(recipient, 200, 1, []byte{})
// 	transaction3, _ := w1.NewTransaction(recipient, 300, 1, []byte{})
// 	transaction4, _ := w1.NewTransaction(recipient, 400, 1, []byte{})
// 	transaction5, _ := w1.NewTransaction(recipient, 500, 1, []byte{})
// 	transaction6, _ := w1.NewTransaction(recipient, 600, 1, []byte{})

// 	transactions := [][]byte{transaction1, transaction2, transaction3, transaction4, transaction5, transaction6}
// 	merkletree, err := blockchain.CreateMerkleTreeFromBytes(transactions)
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	mr := merkletree.MerkleRoot()
// 	log.Println(mr)

// 	vt, err := merkletree.VerifyTree()
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	log.Println("Verify Tree: ", vt)
// }

func hash(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}

// VerifyProof verify proof for value
func VerifyMerkleProof(proof []map[string][]byte, root, value []byte) bool {
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

func TestGenerationMerkleTree(t *testing.T) {
	data := [][]byte{
		[]byte("Buzz"),
		[]byte("Lenny"),
		[]byte("Squeeze"),
		[]byte("Wheezy"),
		[]byte("Jessie"),
		[]byte("Stretch"),
		[]byte("Buster"),
	}
	tree := gomerkle.NewTree(sha256.New())
	tree.AddData(data...)

	err := tree.Generate()
	if err != nil {
		panic(err)
	}
	merkleRoot := tree.Root()

	proof := tree.GetProof(5)
	leaf := tree.GetLeaf(5)

	var listLeaf []string
	var listHash [][]byte
	for _, p := range proof {
		for key, value := range p {
			listLeaf = append(listLeaf, key)
			listHash = append(listHash, value)
		}
	}

	merkleProof := &protobufs.MerkleProof{
		MapLeaf: listLeaf,
		MapHash: listHash,
		Root:    merkleRoot,
		Leaf:    leaf,
	}
	merkleProofByte, _ := proto.Marshal(merkleProof)

	var mp protobufs.MerkleProof
	proto.Unmarshal(merkleProofByte, &mp)

	hashes := mp.GetMapHash()
	var mapProof []map[string][]byte
	for i, key := range mp.GetMapLeaf() {
		m := make(map[string][]byte)
		m[key] = hashes[i]
		mapProof = append(mapProof, m)
	}

	fmt.Println(VerifyMerkleProof(mapProof, merkleRoot, leaf))
}
