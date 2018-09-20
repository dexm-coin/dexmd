package blockchain

import (
	// "encoding/binary"
	"errors"
	// "fmt"
	"math/rand"
	// "os"
	"sort"
	// "github.com/syndtr/goleveldb/leveldb"
	// "github.com/syndtr/goleveldb/leveldb/opt"
	// log "github.com/sirupsen/logrus"
)

// ValidatorsBook is a structure that keeps record of every validator and its stake
type ValidatorsBook struct {
	valsArray map[string]*Validator
}

// Validator is a representation of a validator node
type Validator struct {
	wallet       string
	stake        uint64
	startDynasty int64
	endDynasty   int64
	shard        int64
}

// NewValidatorsBook creates an empty ValidatorsBook object
func NewValidatorsBook() (v *ValidatorsBook) {
	valsArray := make(map[string]*Validator)
	return &ValidatorsBook{valsArray}
}

// CheckIsValidator check if wallet is inside the valsArray
func (v *ValidatorsBook) CheckIsValidator(wallet string) bool {
	if _, ok := v.valsArray[wallet]; ok {
		return true
	}
	return false
}

// CheckDynasty check if the dynasty of wallet are correct
func (v *ValidatorsBook) CheckDynasty(wallet string, currentBlock uint64) bool {
	if _, ok := v.valsArray[wallet]; ok {
		if v.valsArray[wallet].startDynasty+200 < int64(currentBlock) && (v.valsArray[wallet].endDynasty+200 > int64(currentBlock) || v.valsArray[wallet].endDynasty == -1) {
			return true
		}
	}
	return false
}

/*
// ImportValidatorsBook creates a new ValidatorsBook from the content of the database
func ImportValidatorsBook(dbPath string) (v *ValidatorsBook, err error) {
	newVB := NewValidatorsBook()
	var o opt.Options
	o.ErrorIfMissing = true
	db, err := leveldb.OpenFile(dbPath, &o)
	defer db.Close()
	if err != nil {
		return newVB, err
	}

	iter := db.NewIterator(nil, nil)
	// TODO
	for iter.Next() {
		wallet := fmt.Sprintf("%v", iter.Key())
		stake := binary.BigEndian.Uint64((iter.Value()))
		newVB.valsArray[wallet] = Validator{wallet, stake}
		newVB.totalstake += stake
	}
	iter.Release()
	err = iter.Error()
	if err != nil {
		return newVB, err
	}
	return newVB, nil
}

// ExportValidatorsBook creates a new database with the current ValidatorsBook
// If the file already exist, it is erased.
func (v *ValidatorsBook) ExportValidatorsBook(dbPath string) error {
	sort.Sort(v)
	if _, err := os.Stat(dbPath); err == nil {
		os.RemoveAll(dbPath)
	}
	db, err := leveldb.OpenFile(dbPath, nil)
	if err != nil {
		return err
	}
	defer db.Close()
	// TODO
	stakebyte := make([]byte, 8)
	for _, val := range v.valsArray {
		walletbyte := []byte(val.wallet)
		binary.BigEndian.PutUint64(stakebyte, val.stake)
		err = db.Put(walletbyte, stakebyte, nil)
		if err != nil {
			return err
		}
	}
	return nil
}
*/

// AddValidator adds a new validator to the book. If the validator is already
// registered, overwrites its stake with the new one
// Return if the validator already exist or not
func (v *ValidatorsBook) AddValidator(wallet string, stake uint64, dynasty int64) bool {
	if _, ok := v.valsArray[wallet]; ok {
		return true
	}
	v.valsArray[wallet] = &Validator{wallet, stake, dynasty, -1, -1}
	return false
}

// RemoveValidator must be called in case a validator leaves its job
func (v *ValidatorsBook) RemoveValidator(wallet string) error {
	if _, ok := v.valsArray[wallet]; ok {
		delete(v.valsArray, wallet)
		return nil
	}
	return errors.New("Validator " + wallet + " not found")
}

// WithdrawValidator when a withdraw message arrive change the enddynasy of the wallet
func (v *ValidatorsBook) WithdrawValidator(wallet string, r, s []byte, currentBlock int64) error {
	// TODO check signature with r and s
	if _, ok := v.valsArray[wallet]; ok {
		v.valsArray[wallet].endDynasty = currentBlock
		return nil
	}
	return errors.New("Validator " + wallet + " not found")
}

// SetStake is used to update the validator's stake when it changes.
func (v *ValidatorsBook) SetStake(wallet string, addStake uint64) error {
	if _, ok := v.valsArray[wallet]; ok {
		v.valsArray[wallet].stake += addStake
		return nil
	}
	return errors.New("Validator " + wallet + " not found")
}

// GetStake returns the stake for a given wallet.
func (v *ValidatorsBook) GetStake(wallet string) (uint64, error) {
	if _, ok := v.valsArray[wallet]; ok {
		return v.valsArray[wallet].stake, nil
	}
	return 0, errors.New("Validator " + wallet + " not found")
}

// SetShard is used to update the validator's shard when it changes.
func (v *ValidatorsBook) SetShard(wallet string, shard int64) error {
	if _, ok := v.valsArray[wallet]; ok {
		v.valsArray[wallet].shard = shard
		return nil
	}
	return errors.New("Validator " + wallet + " not found")
}

// GetShard is used to get the validator's shard
func (v *ValidatorsBook) GetShard(wallet string) (int64, error) {
	if _, ok := v.valsArray[wallet]; ok {
		return v.valsArray[wallet].shard, nil
	}
	return 0, errors.New("Validator " + wallet + " not found")
}

type simpleValidator struct {
	wallet string
	stake  uint64
}

// ChooseValidator returns a validator's wallet, chosen randomly
// and proportionally to the stake
func (v *ValidatorsBook) ChooseValidator(currentBlock int64) (string, error) {
	rand.Seed(currentBlock)

	totalstake := uint64(0)
	var ss []simpleValidator
	for k, val := range v.valsArray {
		if !v.CheckDynasty(val.wallet, uint64(currentBlock)) {
			continue
		}
		ss = append(ss, simpleValidator{k, val.stake})
		totalstake += val.stake
	}
	if totalstake < 1 {
		return "", errors.New("Not enough stake")
	}
	sort.Slice(ss, func(i, j int) bool {
		return ss[i].stake > ss[j].stake
	})

	level := rand.Float64() * float64(totalstake)
	var counter uint64
	for _, kv := range ss {
		counter += kv.stake
		if float64(counter) >= level {
			return kv.wallet, nil
		}
	}
	return "", errors.New("Validator could not be chosen")
}

// ChooseShard calulate the shard for every validators
// return the shard for a specific wallet
func (v *ValidatorsBook) ChooseShard(seed int64, wallet string) (int64, error) {
	rand.Seed(seed)

	var ss []simpleValidator
	for k, val := range v.valsArray {
		if !v.CheckDynasty(val.wallet, uint64(currentBlock)) {
			continue
		}
		ss = append(ss, simpleValidator{k, val.stake})
	}

	// suffle the validator with a seed
	shardWallet := int64(-1)
	r := rand.New(rand.NewSource(seed))
	perm := r.Perm(len(ss))
	for _, randIndex := range perm {
		shard := rand.Int63n(100)
		randValidator := ss[randIndex]
		if randValidator.wallet == wallet {
			shardWallet = shard
		}
		v.SetShard(randValidator.wallet, shard)
	}
	if shardWallet == -1 {
		return 0, errors.New(wallet + " is not a validator")
	}
	return shardWallet, nil
}
