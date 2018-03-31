package tests

import (
	"testing"

	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
)

func TestWalletGeneration(t *testing.T) {
	w1, err := wallet.GenerateWallet()
	if err != nil {
		t.Error(err)
	}

	w2, err := wallet.GenerateWallet()
	if err != nil {
		t.Error(err)
	}

	addr1, err := w1.GetWallet()
	if err != nil {
		t.Error(err)
	}

	addr2, err := w2.GetWallet()
	if err != nil {
		t.Error(err)
	}

	if addr1 == addr2 {
		t.Error("Wallet collision")
	}
}

func TestTransaction(t *testing.T) {
	w, err := wallet.GenerateWallet()
	if err != nil {
		t.Error(err)
	}

	// Don't do this, it will get rejected by the network
	w.Balance = 1337
	res, err := w.NewTransaction("DexmProofOfBurn", 10, 10)
	if err != nil {
		t.Error(err)
	}

	parsed := &protobufs.Transaction{}
	err = proto.Unmarshal(res, parsed)
	if err != nil {
		t.Error(err)
	}
}

func TestImportExport(t *testing.T) {
	w, err := wallet.GenerateWallet()
	if err != nil {
		t.Error(err)
	}

	w.ExportWallet("/tmp/rand.json")

	w2, err := wallet.ImportWallet("/tmp/rand.json")
	if err != nil {
		t.Error(err)
	}

	_, err = wallet.ImportWallet("/tmp/notafile")
	if err == nil {
		t.Error("Invalid filepath ignored")
	}

	r, err := w.GetWallet()
	if err != nil {
		t.Error(err)
	}

	r1, err := w2.GetWallet()
	if err != nil {
		t.Error(err)
	}

	if r != r1 {
		t.Error("Wallet addresses changed after export")
	}

}

func TestAddress(t *testing.T) {
	w, err := wallet.ImportWallet("testwallet.json")
	if err != nil {
		t.Error(err)
	}

	r, err := w.GetWallet()
	if err != nil {
		t.Error(err)
	}

	if r != "DexmkB1dk7aq2rYz93KaQxscm8FK75A95d2a59a" {
		t.Error("Wallet format changed")
	}
}
