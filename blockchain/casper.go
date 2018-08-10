package blockchain

import (
	"crypto/sha256"
	"fmt"
	"reflect"

	"github.com/dexm-coin/dexmd/wallet"
	"github.com/dexm-coin/protobufs/build/blockchain"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
	"github.com/syndtr/goleveldb/leveldb"
)

// CreateVote : Create a vote based on casper Vote struct
// CasperVote:
// - Source -> hash of the source block
// - Target -> hash of any descendent of s
// - SourceHeight -> height of s
// - TargetHeight -> height of t
// - R, S -> signature of <s, t, h(s), h(t)> with validator private key
func CreateVote(sVote, tVote string, hsVote, htVote uint64, w *wallet.Wallet) protobufs.CasperVote {
	pub, _ := w.GetPubKey()
	data := []byte(sVote + tVote + fmt.Sprintf("%v", hsVote) + fmt.Sprintf("%v", hsVote) + string(pub))
	bhash := sha256.Sum256(data)
	hash := bhash[:]
	rSign, sSign, _ := w.Sign(hash)
	return protobufs.CasperVote{
		Source:       []byte(sVote),
		Target:       []byte(tVote),
		SourceHeight: hsVote,
		TargetHeight: htVote,
		R:            rSign.Bytes(),
		S:            sSign.Bytes(),
		PublicKey:    pub,
	}
}

// CheckpointAgreement : Every checkpoint there should be an agreement of 2/3 of the validators
func CheckpointAgreement(b *Blockchain, votes *[]protobufs.CasperVote) bool {
	mapVote := make(map[string][]uint64)
	for _, vote := range *votes {
		pubKey := string(vote.PublicKey)
		currHeigth := uint64(vote.GetTargetHeight())
		// check if there are multiple votes of the same person
		// in all the heigths votes
		for _, heigths := range mapVote[pubKey] {
			if heigths == currHeigth {
				fmt.Println("slash for ", pubKey)
				// delete the user from the vote counting
				delete(mapVote, pubKey)
				continue
			}
		}
		mapVote[pubKey] = append(mapVote[pubKey], currHeigth)
	}

	if len(mapVote) > 2*len(b.Validators.valsArray)/3 {
		b.CurrentCheckpoint += 100
		return true
	}
	return false
}

// IsJustified : A block is justified if is the root or if it's between 2 checkpoint
func IsJustified(b *Blockchain, block *protobufs.Block) bool {
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

	prevHash := target.PrevHash
	for i := target.Index - 1; i > 0; i-- {
		byteBlock, _ := b.blockDb.Get([]byte(string(i)), nil)
		blocks := &blockchain.Index{}
		proto.Unmarshal(byteBlock, blocks)

		for _, block := range blocks.GetBlocks() {
			byteBlock := []byte(fmt.Sprintf("%v", block))
			bhash := sha256.Sum256(byteBlock)
			currentHash := bhash[:]
			equal := reflect.DeepEqual(prevHash, currentHash)
			if block == source && equal {
				return true
			}
			if equal {
				prevHash = block.PrevHash
			}
		}
	}
	return false
}

// TODO
// GetCanonialBlockchain return the longest chain that is the canonical one
// I should save feald in protobuf blockchain and use checkpoint agreement to know which is the canonical chain
func GetCanonialBlockchain() {
}

// CheckUserVotes check that a validator must not vote within the span of its other votes
// h(s1) < h(s2) < h(t2) < h(t1)
func CheckUserVotes(vote1, vote2 *protobufs.CasperVote) bool {
	// TODO
	// do some check with the signature of the votes that can be false
	if vote1.GetSourceHeight() < vote2.GetSourceHeight() &&
		vote2.GetSourceHeight() < vote2.GetTargetHeight() &&
		vote2.GetTargetHeight() < vote1.GetTargetHeight() {
		return false
	}
	return true
}

// DbCasperVotes contains the leveldb of the casper votes
// TODO use sharding
type DbCasperVotes struct {
	VotesDb *leveldb.DB
}

// NewCasperDb creates a database db for casper votes
func NewCasperDb(dbPath string) (*DbCasperVotes, error) {
	db, err := leveldb.OpenFile(dbPath, nil)
	if err != nil {
		return nil, err
	}
	return &DbCasperVotes{db}, err
}

// SaveCasperVote saves a casper vote inside the DbCasperVotes db
func (cv *DbCasperVotes) SaveCasperVote(vote *protobufs.CasperVote) error {
	// save the CasperVote struct as an array of byte from a string
	return cv.VotesDb.Put([]byte(fmt.Sprintf("%v", vote)), nil, nil)
}
