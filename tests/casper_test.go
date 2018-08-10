package tests

import (
	"testing"

	"github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
)

func TestCheckpointAgreement(t *testing.T) {
	b, err := blockchain.NewBlockchain("/tmp/blockchain", 0)
	if err != nil {
		t.Error(err)
	}

	w1, err := wallet.GenerateWallet()
	if err != nil {
		t.Error(err)
	}
	addr1, err := w1.GetWallet()
	if err != nil {
		t.Error(err)
	}
	if !wallet.IsWalletValid(addr1) {
		t.Error("Generated wallet is invalid")
	}

	vote1 := blockchain.CreateVote("casperVote1Source", "casperVote1Target", 0, 2, w1)
	vote2 := blockchain.CreateVote("casperVote2Source", "casperVote2Target", 0, 3, w1)
	vote3 := blockchain.CreateVote("casperVote3Source", "casperVote3Target", 0, 1, w1)
	votes := []protobufs.CasperVote{vote1, vote2, vote3}
	blockchain.CheckpointAgreement(b, &votes)
}
