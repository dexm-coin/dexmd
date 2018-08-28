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
func CheckpointAgreement(b *Blockchain, votes *[]protobufs.CasperVote, source, target *protobufs.Block) bool {
	if !IsVoteValid(b, source, target) {
		log.Error("Source and target and not valid")
		return false
	}
	// The block before the currenctCheckpoint are already justified so there is no need of a checkpoint,
	// also the checkpoint is every 100 blocks
	if target.GetIndex() <= b.CurrentCheckpoint && target.GetIndex()%100 != 0 {
		return false
	}

	mapVote := make(map[string][]uint64)
	var userToRemove []string
	for _, vote := range *votes {
		if vote.GetSourceHeight() != source.GetIndex() {
			continue
		}
		currTargetHeight := vote.GetTargetHeight()
		if currTargetHeight != target.GetIndex() {
			continue
		}

		pubKey := string(vote.PublicKey)
		// check if there are multiple votes of the same person
		// in all the heigths votes
		for _, heigths := range mapVote[pubKey] {
			if heigths == currTargetHeight {
				userToRemove = append(userToRemove, pubKey)
				continue
			}
		}
		mapVote[pubKey] = append(mapVote[pubKey], currTargetHeight)
	}

	// delete the users from the vote counting
	for _, user := range userToRemove {
		fmt.Println("slash for ", user)
		delete(mapVote, user)
	}

	// TODO with the forks this is wrong
	if len(mapVote) > 2*len(b.Validators.valsArray)/3 {
		b.CurrentCheckpoint = target.GetIndex()
		return true
	}
	return false
}

// IsJustified : A block is justified if is the root or if it's between 2 checkpoint
func IsJustified(b *Blockchain, block *protobufs.Block) bool {
	index := block.GetIndex()
	if index > b.CurrentCheckpoint {
		return false
	}
	return true
}

// IsVoteValid check if s is an ancestor of t in the chain
func IsVoteValid(b *Blockchain, source, target *protobufs.Block) bool {
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
			// check untill the fist checkpoint
			if IsJustified(b, block) {
				return false
			}

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

// CheckUserVote check that a validator must not vote within the span of its other votes
// return if its valid and if the signature are right
func CheckUserVote(vote1, vote2 *protobufs.CasperVote) (bool, bool) {
	// check the signature of the votes
	dataVote1 := []byte(string(vote1.Source) + string(vote1.Target) + fmt.Sprintf("%v", vote1.SourceHeight) + fmt.Sprintf("%v", vote1.TargetHeight) + string(vote1.PublicKey))
	bhash1 := sha256.Sum256(dataVote1)
	hash1 := bhash1[:]
	dataVote2 := []byte(string(vote2.Source) + string(vote2.Target) + fmt.Sprintf("%v", vote2.SourceHeight) + fmt.Sprintf("%v", vote2.TargetHeight) + string(vote2.PublicKey))
	bhash2 := sha256.Sum256(dataVote2)
	hash2 := bhash2[:]

	verifyVote1, err1 := wallet.SignatureValid(vote1.GetPublicKey(), vote1.GetR(), vote1.GetS(), hash1)
	verifyVote2, err2 := wallet.SignatureValid(vote2.GetPublicKey(), vote2.GetR(), vote2.GetS(), hash2)
	if err1 != nil || err2 != nil {
		log.Error("Check SignatureValid failed")
		return false, false
	}
	if string(vote1.PublicKey) != string(vote2.PublicKey) || !verifyVote1 || !verifyVote2 {
		log.Warning("slashing to the client that have request the CheckUserVote becuase the votes are fake")
		return false, true
	}

	// h(s1) < h(s2) < h(t2) < h(t1)
	if vote1.GetSourceHeight() < vote2.GetSourceHeight() &&
		vote2.GetSourceHeight() < vote2.GetTargetHeight() &&
		vote2.GetTargetHeight() < vote1.GetTargetHeight() {
		return false, false
	}
	return true, false
}

// DbCasperVotes contains the leveldb of the casper votes
// TODO use sharding
type DbCasperVotes struct {
	VotesDb *leveldb.DB
	Index   int
}

// NewCasperDb creates a database db for casper votes
func NewCasperDb(dbPath string) (*DbCasperVotes, error) {
	db, err := leveldb.OpenFile(dbPath, nil)
	if err != nil {
		return nil, err
	}
	return &DbCasperVotes{db, 0}, err
}

// SaveCasperVote saves a casper vote inside the DbCasperVotes db
func (cv *DbCasperVotes) SaveCasperVote(vote *protobufs.CasperVote) error {
	cv.Index++
	res, err := proto.Marshal(vote)
	if err != nil {
		return err
	}
	// save the CasperVote struct as an array of byte from a string
	return cv.VotesDb.Put([]byte(string(cv.Index)), res, nil)
}

// GetCasperVote get a casper vote inside the DbCasperVotes db
func (cv *DbCasperVotes) GetCasperVote(index int) (protobufs.CasperVote, error) {
	oldVote, err := cv.VotesDb.Get([]byte(string(index)), nil)

	vote := protobufs.CasperVote{}
	if err == nil {
		proto.Unmarshal(oldVote, &vote)
	}

	// return cv.VotesDb.Get([]byte(string(index)), nil)
	return vote, err
}
