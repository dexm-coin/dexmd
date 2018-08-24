package blockchain

import (
	"reflect"
	"crypto/sha256"
	"log"

	"github.com/cbergoon/merkletree"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
)

//TestContent implements the Content interface provided by merkletree and represents the content stored in the tree.
type TestContent struct {
	x protobufs.Transaction
}

//CalculateHash hashes the values of a TestContent
func (t TestContent) CalculateHash() ([]byte, error) {
	h := sha256.New()
	result, err := proto.Marshal(&t.x)
	if err != nil {
		return nil, err
	}
	if _, err := h.Write([]byte(result)); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

//Equals tests for equality of two Contents
func (t TestContent) Equals(other merkletree.Content) (bool, error) {
	// hashT, _ := t.CalculateHash()
	// hashOther, _ := other.CalculateHash()
	return reflect.DeepEqual(t.x, other.(TestContent).x), nil
}

// CreateMerkleTreeFromBytes take an array of bytes array, convert in transaction and add them all in a Content list to create a MerkleTree
func CreateMerkleTreeFromBytes(bytesTransaction [][]byte) (*merkletree.MerkleTree, error) {
	var list []merkletree.Content
	for _, bTransaction := range bytesTransaction {
		transaction := protobufs.Transaction{}
		proto.Unmarshal(bTransaction, &transaction)
		list = append(list, TestContent{x: transaction})
	}

	t, err := merkletree.NewTree(list)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	return t, err
}
