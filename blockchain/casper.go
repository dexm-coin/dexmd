package blockchain

import (
	"crypto/sha256"
	"fmt"

	"github.com/dexm-coin/dexmd/wallet"
	"github.com/dexm-coin/protobufs/build/blockchain"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
)

// Create a vote based on casper Vote struct
// CasperVote:
// - Source -> hash of the source checkpoint
// - Target -> hash of any descendent of s
// - SourceHeight -> height of s
// - TargetHeight -> height of t
// - R, S -> signature of <s, t, h(s), h(t)> with validator private key
func createVote(sVote, tVote string, hsVote, htVote uint64, w *wallet.Wallet) blockchain.CasperVote {
	data := []byte(sVote + tVote + fmt.Sprintf("%v", hsVote) + fmt.Sprintf("%v", hsVote))
	bhash := sha256.Sum256(data)
	hash := bhash[:]
	rSign, sSign, _ := w.Sign(hash)
	return blockchain.CasperVote{
		Source:       []byte(sVote),
		Target:       []byte(tVote),
		SourceHeight: hsVote,
		TargetHeight: htVote,
		R:            rSign.Bytes(),
		S:            sSign.Bytes(),
	}
}

// Every checkpoint there should be an agreement of
// 2/3 of the validators
func checkpointAgreement() {

}

// A checkpoint c is called justified if it is the root,
// or if there exists a supermajority link c to any of its
// direct children c' in the checkpoint tree, where c' is justified
func isJustified() {

}

// A checkpoint c is called finalized if it is justified
// and there is a supermajority link from c to any of its
// direct children in the checkpoint tree
func isFinalized() {

}

// IsVoteValid check if s is an ancestor of t in the chain
func IsVoteValid(b *Blockchain, source *protobufs.Block, target *protobufs.Block) bool {
	if len(b.Validators.valsArray) < 1 {
		log.Error("No validators")
		return false
	}
	for i := target.Index - 1; i > 0; i-- {
		byteBlock, _ := b.blockDb.Get([]byte(string(i)), nil)
		blocks := &blockchain.Index{}
		proto.Unmarshal(byteBlock, blocks)

		for _, block := range blocks.GetBlocks() {
			if block == source {
				return true
			}
		}
	}
	return false
}
