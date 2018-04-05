package tests

import (
	"testing"

	"github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
)

func TestBlockValidation(t *testing.T) {
	b, err := blockchain.NewBlockchain("/tmp/blockchain", 0)
	if err != nil {
		t.Error(err)
	}

	// Generate a fake genesis block for testing
	w1, _ := wallet.GenerateWallet()
	w2, _ := wallet.GenerateWallet()

	recipient, _ := w2.GetWallet()
	transaction, _ := w1.NewTransaction(recipient, 1000, 50)

	parsed := &protobufs.Transaction{}
	err = proto.Unmarshal(transaction, parsed)
	if err != nil {
		t.Error(err)
	}

	genesis := protobufs.Block{
		Index:        1,
		Timestamp:    0,
		PrevHash:     []byte{0},
		Miner:        recipient,
		Transactions: []*protobufs.Transaction{parsed},
	}

	b.ValidateBlock(&genesis)
}
