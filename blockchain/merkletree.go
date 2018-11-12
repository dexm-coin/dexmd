package blockchain

import (
	"crypto/sha256"

	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	"github.com/onrik/gomerkle"
)

// GenerateMerkleTree generate a merkletree and return its root
func GenerateMerkleTree(transactions []*protobufs.Transaction, receiptsContract []*protobufs.Receipt) ([]byte, error) {
	var dataReceipt [][]byte
	// For every transaction create its receipt and use it to create the tree
	for _, t := range transactions {
		r := &protobufs.Receipt{
			Sender:    wallet.BytesToAddress(t.GetSender(), t.GetShard()),
			Recipient: t.GetRecipient(),
			Amount:    t.GetAmount(),
			Nonce:     t.GetNonce(),
		}
		rByte, _ := proto.Marshal(r)
		dataReceipt = append(dataReceipt, rByte)
	}

	for _, r := range receiptsContract {
		rByte, _ := proto.Marshal(r)
		dataReceipt = append(dataReceipt, rByte)
	}

	treeReceipt := gomerkle.NewTree(sha256.New())
	treeReceipt.AddData(dataReceipt...)
	err := treeReceipt.Generate()
	if err != nil {
		return nil, err
	}
	merkleRootReceipt := treeReceipt.Root()

	return merkleRootReceipt, nil
}
