package blockchain

import (
	"crypto/sha256"
	"fmt"

	"github.com/dexm-coin/dexmd/wallet"
	"github.com/dexm-coin/protobufs/build/network"
)

// Vote is the struct of a vote in casper
type Vote struct {
	s         string            // hash of the source checkpoint
	t         string            // hash of the descendent of s
	hS        uint64            // height of s
	hT        uint64            // height of t
	signature network.Signature // signature of <s, t, h(s), h(t)> with validator private key
}

// Create a vote based on casper Vote struct
func createVote(sVote, tVote string, hsVote, htVote uint64, w *wallet.Wallet) Vote {
	data := []byte(sVote + tVote + fmt.Sprintf("%v", hsVote) + fmt.Sprintf("%v", hsVote))
	bhash := sha256.Sum256(data)
	hash := bhash[:]
	r, s, _ := w.Sign(hash)
	pub, _ := w.GetPubKey()
	return Vote{sVote, tVote, hsVote, htVote, network.Signature{
		Pubkey: pub,
		R:      r.Bytes(),
		S:      s.Bytes(),
		Data:   hash,
	}}
}

// Every checkpoint there should be an agreement of
// 2/3 of the validators
func checkpointAgreement() {

}

// A checkpoint c is called justified if it is the root,
// or if there exists a supermajority link c to any of its
// direct children c' in the checkpoint tree, where c' is
// justified
func isJustified() {

}

// A checkpoint c is called finalized if it is justified
// and there is a supermajority link from c to any of its
// direct children in the checkpoint tree
func isFinalized() {

}
