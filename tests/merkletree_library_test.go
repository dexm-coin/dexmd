package tests

import (
	"testing"

	blockchain "github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/wallet"
	log "github.com/sirupsen/logrus"
)

func TestGenerationMerkleTree(t *testing.T) {
	// Generate a fake genesis block for testing
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

	transactions := [][]byte{transaction1, transaction2, transaction3, transaction4, transaction5, transaction6}
	merkletree, err := blockchain.CreateMerkleTreeFromBytes(transactions)
	if err != nil {
		t.Error(err)
	}

	mr := merkletree.MerkleRoot()
	log.Println(mr)

	vt, err := merkletree.VerifyTree()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Verify Tree: ", vt)
}
