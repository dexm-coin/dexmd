package tests

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"

	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
)

type Node struct {
	Parent *Node
	Left   *Node
	Right  *Node
	leaf   bool
	dup    bool
	Hash   []byte
}

func NewTree(cs []*Node) (*protobufs.MerkleTree, error) {
	root, leafs, err := build(cs)
	if err != nil {
		return nil, err
	}

	var hashes [][]byte
	var areLeafs []bool
	for _, node := range leafs {
		hashes = append(hashes, node.Hash)
		areLeafs = append(areLeafs, node.leaf)
	}

	return &protobufs.MerkleTree{
		Hashes:     hashes,
		MerkleRoot: root.Hash,
		Leafs:      areLeafs,
	}, nil
}

func build(cs []*Node) (*Node, []*Node, error) {
	if len(cs) == 0 {
		return nil, nil, errors.New("error: cannot construct tree with no content")
	}
	var leafs []*Node
	for _, c := range cs {
		hash := c.Hash
		leafs = append(leafs, &Node{
			Hash: hash,
			leaf: true,
		})
	}
	if len(leafs)%2 == 1 {
		duplicate := &Node{
			Hash: leafs[len(leafs)-1].Hash,
			leaf: true,
			dup:  true,
		}
		leafs = append(leafs, duplicate)
	}
	root, err := buildIntermediate(leafs)
	if err != nil {
		return nil, nil, err
	}

	return root, leafs, nil
}

//buildIntermediate is a helper function that for a given list of leaf nodes, constructs
//the intermediate and root levels of the tree. Returns the resulting root node of the tree.
func buildIntermediate(nl []*Node) (*Node, error) {
	var nodes []*Node
	for i := 0; i < len(nl); i += 2 {
		h := sha256.New()
		var left, right int = i, i + 1
		if i+1 == len(nl) {
			right = i
		}
		chash := append(nl[left].Hash, nl[right].Hash...)
		if _, err := h.Write(chash); err != nil {
			return nil, err
		}
		n := &Node{
			Left:  nl[left],
			Right: nl[right],
			Hash:  h.Sum(nil),
		}
		nodes = append(nodes, n)
		nl[left].Parent = n
		nl[right].Parent = n
		if len(nl) == 2 {
			return n, nil
		}
	}
	return buildIntermediate(nodes)
}

func calculateHashTransaction(t *protobufs.Transaction) []byte {
	data, _ := proto.Marshal(t)
	bhash := sha256.Sum256(data)
	return bhash[:]
}

func TestMerkeTree(t *testing.T) {
	w1, _ := wallet.GenerateWallet()
	w2, _ := wallet.GenerateWallet()

	recipient, _ := w2.GetWallet()

	w1.Balance = 10000
	transaction1, _ := w1.NewTransaction(recipient, 100, 1, []byte{})
	transaction2, _ := w1.NewTransaction(recipient, 200, 1, []byte{})
	transaction3, _ := w1.NewTransaction(recipient, 300, 1, []byte{})
	transaction4, _ := w1.NewTransaction(recipient, 400, 1, []byte{})
	transaction5, _ := w1.NewTransaction(recipient, 500, 1, []byte{})
	transaction6, _ := w1.NewTransaction(recipient, 600, 1, []byte{})
	transaction7, _ := w1.NewTransaction(recipient, 700, 1, []byte{})
	transaction8, _ := w1.NewTransaction(recipient, 800, 1, []byte{})

	transactionsByte := [][]byte{transaction1, transaction2, transaction3, transaction4, transaction5, transaction6, transaction7, transaction8}

	var transactions []*protobufs.Transaction
	for _, bTransaction := range transactionsByte {
		transaction := protobufs.Transaction{}
		proto.Unmarshal(bTransaction, &transaction)
		transactions = append(transactions, &transaction)
	}

	var cs []*Node
	for _, t := range transactions {
		hash := calculateHashTransaction(t)
		cs = append(cs, &Node{Hash: hash})
	}
	merkleTree, _ := NewTree(cs)
	fmt.Println(merkleTree)
}
