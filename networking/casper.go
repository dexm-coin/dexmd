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
func (cs *ConnectionStore) CheckpointAgreement(SourceHeight, TargetHeight uint64) bool {
	mapVote := make(map[string]bool)
	var userToRemove []string
	for _, vote := range receivedVotes {
		if vote.GetSourceHeight() != SourceHeight {
			continue
		}
		currTargetHeight := vote.GetTargetHeight()
		if currTargetHeight != TargetHeight {
			continue
		}

		pubKey := vote.PublicKey
		if !cs.beaconChain.Validators.CheckDynasty(pubKey, cs.shardChain.CurrentBlock) {
			log.Error("Vote not valid based on dynasty of validator")
			continue
		}
		if !IsVoteValid(cs.shardChain, vote.GetSource(), vote.GetTarget(), vote.GetTargetHeight()) {
			log.Error("Source and target and not valid")
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

	if len(mapVote) > 2*cs.beaconChain.Validators.LenValidators()/3 {
		// delete all the votes only if 2/3 of validators agree
		// so h(s1) < h(s2) < h(t2) < h(t1) is valid
		receivedVotes = []*protobufs.CasperVote{}

		cs.shardChain.CurrentCheckpoint = TargetHeight
		return true
	}
	return false
}

// IsJustified : A block is justified if is the root or if it's between 2 checkpoint
func IsJustified(b *blockchain.Blockchain, index uint64) bool {
	if index > b.CurrentCheckpoint {
		return false
	}
	return true
}

// IsVoteValid check if s is an ancestor of t in the chain
func IsVoteValid(b *blockchain.Blockchain, sourceHash, targetHash []byte, TargetHeight uint64) bool {
	// if len(b.Validators.valsArray) < 1 {
	// 	log.Error("No validators")
	// 	return false
	// }

	targetByte, _ := b.GetBlock(TargetHeight)
	target := &protobufs.Block{}
	proto.Unmarshal(targetByte, target)
	prevHash := target.GetPrevHash()

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
		}
		// check untill the fist checkpoint
		if IsJustified(b, i) {
			return false
		}
	}
	return false
}

/*
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
}*/

// AddVote add a vote in receivedVotes and put it on the db
func (cs *ConnectionStore) AddVote(vote *protobufs.CasperVote) error {
	if !cs.beaconChain.Validators.CheckIsValidator(vote.GetPublicKey()) {
		return nil
	}
	receivedVotes = append(receivedVotes, vote)

	res, err := proto.Marshal(vote)
	if err != nil {
		return err
	}
	return cs.shardChain.CasperVotesDb.Put([]byte(string(cs.shardChain.CurrentVote)), res, nil)
}

// GetCasperVote get a casper vote inside CasperVotesDb
func (cs *ConnectionStore) GetCasperVote(index int) (protobufs.CasperVote, error) {
	oldVote, err := cs.shardChain.CasperVotesDb.Get([]byte(strconv.Itoa(index)), nil)

	vote := protobufs.CasperVote{}
	if err == nil {
		proto.Unmarshal(oldVote, &vote)
	}
	return vote, err
}
