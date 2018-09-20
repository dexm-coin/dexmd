package blockchain

import (
	"crypto/sha256"
	"log"
	"reflect"

	"github.com/cbergoon/merkletree"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
)

type TransactionContent struct {
	x *protobufs.Transaction
}

func (t TransactionContent) CalculateHash() ([]byte, error) {
	h := sha256.New()
	result, err := proto.Marshal(t.x)
	if err != nil {
		return nil, err
	}
	if _, err := h.Write([]byte(result)); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
func (t TransactionContent) Equals(other merkletree.Content) (bool, error) {
	return reflect.DeepEqual(t.x, other.(TransactionContent).x), nil
}

type ReceiptContent struct {
	x *protobufs.Receipt
}

func (t ReceiptContent) CalculateHash() ([]byte, error) {
	h := sha256.New()
	result, err := proto.Marshal(t.x)
	if err != nil {
		return nil, err
	}
	if _, err := h.Write([]byte(result)); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
func (t ReceiptContent) Equals(other merkletree.Content) (bool, error) {
	return reflect.DeepEqual(t.x, other.(ReceiptContent).x), nil
}

// CreateMerkleTreeFromBytes take an array of bytes array, convert in transaction and add them all in a Content list to create a MerkleTree
func CreateMerkleTreeFromBytes(bytesTransaction [][]byte) (*merkletree.MerkleTree, error) {
	var list []merkletree.Content
	for _, bTransaction := range bytesTransaction {
		transaction := protobufs.Transaction{}
		proto.Unmarshal(bTransaction, &transaction)
		list = append(list, TransactionContent{x: &transaction})
	}

	t, err := merkletree.NewTree(list)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	return t, err
}

// CreateMerkleTrees create 2 merkle trees, one for the transaction and one for the receipt of the transaction
func CreateMerkleTrees(transactions []*protobufs.Transaction) ([]byte, []byte, error) {
	var listTransaction []merkletree.Content
	var listReceipt []merkletree.Content
	for _, t := range transactions {
		listTransaction = append(listTransaction, TransactionContent{x: t})
		// receipt := &protobufs.Receipt{string(t.GetSender()), t.GetRecipient(), t.GetAmount()}
		// listReceipt = append(listReceipt, ReceiptContent{x: receipt})
	}

	MerkleTreeTransaction, err := merkletree.NewTree(listTransaction)
	if err != nil {
		log.Fatal(err)
		return nil, nil, err
	}
	vt, err := MerkleTreeTransaction.VerifyTree()
	if err != nil || !vt {
		log.Fatal(err)
		return nil, nil, err
	}

	MerkleTreeReceipt, err := merkletree.NewTree(listReceipt)
	if err != nil {
		log.Fatal(err)
		return nil, nil, err
	}
	vt, err = MerkleTreeReceipt.VerifyTree()
	if err != nil || !vt {
		log.Fatal(err)
		return nil, nil, err
	}

	return MerkleTreeTransaction.MerkleRoot(), MerkleTreeReceipt.MerkleRoot(), nil
}
