package networking

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strconv"

	"github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
)

var receivedVotes []*protobufs.CasperVote

// CreateVote : Create a vote based on casper Vote struct
// CasperVote:
// - Source -> hash of the source block
// - Target -> hash of any descendent of s
// - SourceHeight -> height of s
// - TargetHeight -> height of t
// - R, S -> signature of <s, t, h(s), h(t)> with validator private key
func CreateVote(sVote, tVote []byte, hsVote, htVote uint64, w *wallet.Wallet) protobufs.CasperVote {
	wal, err := w.GetWallet()
	if err != nil {
		log.Fatal(err)
	}
	data := []byte(fmt.Sprintf("%v", sVote) + fmt.Sprintf("%v", tVote) + fmt.Sprintf("%v", hsVote) + fmt.Sprintf("%v", hsVote) + wal)
	bhash := sha256.Sum256(data)
	hash := bhash[:]
	rSign, sSign, _ := w.Sign(hash)
	return protobufs.CasperVote{
		Source:       sVote,
		Target:       tVote,
		SourceHeight: hsVote,
		TargetHeight: htVote,
		R:            rSign.Bytes(),
		S:            sSign.Bytes(),
		PublicKey:    wal,
	}
}

// CheckpointAgreement : Every checkpoint there should be an agreement of 2/3 of the validators
func (cs *ConnectionStore) CheckpointAgreement(SourceHeight, TargetHeight uint64, currentShard uint32) bool {
	mapVote := make(map[string]bool)
	var userToRemove []string
	// currentShard := uint32(cs.identity.GetShardWallet())
	for _, vote := range receivedVotes {
		if vote.GetSourceHeight() != SourceHeight {
			continue
		}
		currTargetHeight := vote.GetTargetHeight()
		if currTargetHeight != TargetHeight {
			continue
		}

		pubKey := vote.PublicKey
		shardSender, err := cs.beaconChain.Validators.GetShard(pubKey)
		if err != nil {
			log.Error(err)
			continue
		}

		if shardSender != currentShard {
			log.Error("Validator is from a different shard")
			continue
		}
		if !cs.beaconChain.Validators.CheckDynasty(pubKey, cs.shardsChain[currentShard].CurrentBlock) {
			log.Error("Vote not valid based on dynasty of validator")
			continue
		}
		if !IsVoteValid(cs.shardsChain[currentShard], vote.GetSource(), vote.GetTarget(), vote.GetTargetHeight()) {
			log.Error("Source and target are not valid")
			continue
		}

		// check if there are multiple votes of the same person
		if _, ok := mapVote[pubKey]; ok {
			userToRemove = append(userToRemove, pubKey)
		}
		mapVote[pubKey] = true
	}

	// delete the users from the vote counting
	for _, user := range userToRemove {
		fmt.Println("slash for ", user)
		delete(mapVote, user)
	}

	if float64(len(mapVote)) > float64(2*cs.beaconChain.Validators.LenValidators(currentShard)/3) {
		// delete all the votes only if 2/3 of validators agree
		// so h(s1) < h(s2) < h(t2) < h(t1) is valid
		receivedVotes = []*protobufs.CasperVote{}

		cs.shardsChain[currentShard].CurrentCheckpoint = TargetHeight
		return true
	}
	return false
}

// IsJustified : A block is justified if is the root or if it's between 2 checkpoint
func IsJustified(currentCheckpoint uint64, index uint64) bool {
	if index > currentCheckpoint {
		return false
	}
	return true
}

// IsVoteValid check if s is an ancestor of t in the chain
func IsVoteValid(b *blockchain.Blockchain, sourceHash, targetHash []byte, TargetHeight uint64) bool {
	targetByte, _ := b.GetBlock(TargetHeight)
	target := &protobufs.Block{}
	proto.Unmarshal(targetByte, target)
	prevHash := target.GetPrevHash()

	// from TargetHeight to the nearest checkpoint, check if the hash of source is beflow the hash of target
	for i := TargetHeight - 1; i >= 0; i-- {
		byteBlock, _ := b.GetBlock(i)
		block := &protobufs.Block{}
		proto.Unmarshal(byteBlock, block)

		bhash := sha256.Sum256(byteBlock)
		currentHash := bhash[:]
		equal := reflect.DeepEqual(prevHash, currentHash)
		isTheSource := reflect.DeepEqual(currentHash, sourceHash)
		if isTheSource && equal {
			return true
		}
		if equal {
			if len(block.PrevHash) < 1 {
				continue
			}
			prevHash = block.GetPrevHash()
		}
		// check untill the first checkpoint
		if IsJustified(b.CurrentCheckpoint, i) {
			return false
		}
	}
	return false
}

// AddVote add a vote in receivedVotes and put it on the db
func (cs *ConnectionStore) AddVote(vote *protobufs.CasperVote, shard uint32) error {
	if !cs.beaconChain.Validators.CheckIsValidator(vote.GetPublicKey()) {
		return nil
	}
	receivedVotes = append(receivedVotes, vote)

	res, err := proto.Marshal(vote)
	if err != nil {
		return err
	}
	return cs.shardsChain[shard].CasperVotesDb.Put([]byte(string(cs.shardsChain[shard].CurrentVote)), res, nil)
}

// GetCasperVote get a casper vote inside CasperVotesDb
func (cs *ConnectionStore) GetCasperVote(index int, shard uint32) (protobufs.CasperVote, error) {
	oldVote, err := cs.shardsChain[shard].CasperVotesDb.Get([]byte(strconv.Itoa(index)), nil)

	vote := protobufs.CasperVote{}
	if err == nil {
		proto.Unmarshal(oldVote, &vote)
	}
	return vote, err
}
