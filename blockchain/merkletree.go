package blockchain

import (
	"crypto/sha256"

	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	"github.com/onrik/gomerkle"
)

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
