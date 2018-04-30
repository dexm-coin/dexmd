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
// - Source -> hash of the source block
// - Target -> hash of any descendent of s
// - SourceHeight -> height of s
// - TargetHeight -> height of t
// - R, S -> signature of <s, t, h(s), h(t)> with validator private key
func createVote(sVote, tVote string, hsVote, htVote uint64, w *wallet.Wallet) blockchain.CasperVote {
	pub, _ := w.GetPubKey()
	data := []byte(sVote + tVote + fmt.Sprintf("%v", hsVote) + fmt.Sprintf("%v", hsVote) + string(pub))
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
		PublicKey:    pub,
	}
}

// Every checkpoint there should be an agreement of 2/3 of the validators
func checkpointAgreement(b *Blockchain, votes *[]blockchain.CasperVote) bool {
	mapVote := make(map[string]bool)
	var newVotes []blockchain.CasperVote
	for _, vote := range *votes {
		pubKey := string(vote.PublicKey)
		// It will check only if there are duplicates vote
		if _, ok := mapVote[pubKey]; ok {
			fmt.Println("ban", pubKey)
			continue
		}
		mapVote[pubKey] = true
		newVotes = append(newVotes, vote)
	}

	if len(newVotes) > 2*len(b.Validators.valsArray)/3 {
		b.CurrentCheckpoint += 100
		return true
	}
	return false
}

// A block is justified if is the root or if it's between 2 checkpoint
func isJustified(b *Blockchain, block *protobufs.Block) bool {
	index := block.GetIndex()
	if index > b.CurrentCheckpoint && index%100 != 0 {
		return false
	}
	return true
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
