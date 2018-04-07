package blockchain

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sort"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// ValidatorsBook is a structure that keeps record of every validator and its stake
type ValidatorsBook struct {
	totalstake uint64
	valsArray  []Validator
}

// Validator is a representation of a validator node
type Validator struct {
	wallet string
	stake  uint64
}

// NewValidatorsBook creates an empty ValidatorsBook object
func NewValidatorsBook() (v ValidatorsBook) {
	var newVB ValidatorsBook
	newVB.totalstake = 0
	return newVB
}

// ImportValidatorsBook creates a new ValidatorsBook from the content of the database
func ImportValidatorsBook(dbPath string) (v *ValidatorsBook, err error) {
	newVB := NewValidatorsBook()
	var o opt.Options
	o.ErrorIfMissing = true
	db, err := leveldb.OpenFile(dbPath, &o)
	defer db.Close()
	if err != nil {
		return &newVB, err
	}

	iter := db.NewIterator(nil, nil)
	for iter.Next() {
		wallet := string(iter.Key())
		stake := binary.BigEndian.Uint64((iter.Value()))
		newVB.valsArray = append(newVB.valsArray, Validator{wallet, stake})
		newVB.totalstake += stake
	}
	iter.Release()
	err = iter.Error()
	if err != nil {
		return &newVB, err
	}
	sort.Sort(&newVB)
	return &newVB, nil
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

// AddValidator adds a new validator to the book. If the validator is already
// registered, overwrites its stake with the new one
func (v *ValidatorsBook) AddValidator(wallet string, stake uint64) {
	for i, val := range v.valsArray {
		if val.wallet == wallet {
			v.totalstake += stake - val.stake
			v.valsArray[i].stake = stake
			return
		}
	}
	newVal := Validator{wallet, stake}
	v.valsArray = append(v.valsArray, newVal)
	v.totalstake += stake
}

// RemoveValidator must be called in case a validator leaves its job
func (v *ValidatorsBook) RemoveValidator(wallet string) error {
	for i, val := range v.valsArray {
		if val.wallet == wallet {
			v.totalstake -= val.stake
			v.valsArray = append(v.valsArray[:i], v.valsArray[i+1:]...)
			return nil
		}
	}
	return errors.New("Validator " + wallet + " not found")
}

// SetStake is used to update the validator's stake when it changes.
func (v *ValidatorsBook) SetStake(wallet string, addStake uint64) error {
	for i, val := range v.valsArray {
		if val.wallet == wallet {
			v.totalstake += addStake
			v.valsArray[i].stake += addStake
			return nil
		}
	}
	return errors.New("Validator " + wallet + " not found")
}

// GetStake returns the stake for a given wallet.
func (v *ValidatorsBook) GetStake(wallet string) (stake uint64, err error) {
	for _, val := range v.valsArray {
		if val.wallet == wallet {
			return val.stake, nil
		}
	}
	return 0, errors.New("Validator " + wallet + " not found")
}

// ChooseValidator returns a validator's wallet, chosen randomly
// and proportionally to the stake
func (v *ValidatorsBook) ChooseValidator(seed int64) (luckyone string, err error) {
	sort.Sort(v)
	rand.Seed(seed)
	level := rand.Float64() * float64(v.totalstake)
	var counter uint64
	for _, val := range v.valsArray {
		counter += val.stake
		if float64(counter) >= level {
			return val.wallet, nil
		}
	}
	return "", errors.New("Validator could not be chosen")
}

// Len is used in sorting
func (v *ValidatorsBook) Len() int {
	return len(v.valsArray)
}

// Swap is used in sorting
func (v *ValidatorsBook) Swap(i, j int) {
	v.valsArray[i], v.valsArray[j] = v.valsArray[j], v.valsArray[i]
}

// Less is used in sorting. Sorts in descending order for stake
func (v *ValidatorsBook) Less(i, j int) bool {
	return v.valsArray[i].stake > v.valsArray[j].stake
}

// Repr is a debug function, prints the internal state
func (v *ValidatorsBook) Repr() {
	for _, val := range v.valsArray {
		fmt.Printf("%s - %d\n", val.wallet, val.stake)
	}
	fmt.Printf("tot: %d\n", v.totalstake)
}

// IsEqualTo checks every entry of the book to determine if they are equal
func (v *ValidatorsBook) IsEqualTo(v2 *ValidatorsBook) bool {
	if v.Len() != v2.Len() {
		return false
	}
	for _, val := range v.valsArray {
		v2stake, err := v2.GetStake(val.wallet)
		if err != nil {
			return false
		}
		if val.stake != v2stake {
			return false
		}
	}
	return true
}

// AddValidatorFast is a version of AddValidator that doesn't check if the validator exists
// Is way faster and used for testing purposes only.
func (v *ValidatorsBook) AddValidatorFast(wallet string, stake uint64) {
	newVal := Validator{wallet, stake}
	v.valsArray = append(v.valsArray, newVal)
	v.totalstake += stake
}
