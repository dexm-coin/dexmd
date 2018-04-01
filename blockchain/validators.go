package blockchain

import (
	"errors"
	"math/rand"
)

// ValidatorsBook is a structure that keeps record of every validator and its stake
type ValidatorsBook struct {
	totalstake uint32
	valsArray  []Validator
}

// Validator is a representation of a validator node
type Validator struct {
	wallet string
	stake  uint32
}

// AddValidator adds a new validator to the book
func (v *ValidatorsBook) AddValidator(wallet string, stake uint32) error {
	for _, val := range v.valsArray {
		if val.wallet == wallet {
			return errors.New("Validator already present")
		}
	}
	newVal := Validator{wallet, stake}
	v.valsArray = append(v.valsArray, newVal)
	v.totalstake += stake
	return nil
}

// RemoveValidator must be called in case a validator leaves its job
func (v *ValidatorsBook) RemoveValidator(wallet string) error {
	for i, val := range v.valsArray {
		if val.wallet == wallet {
			v.valsArray = append(v.valsArray[:i], v.valsArray[i+1:]...)
			return nil
		}
	}
	return errors.New("Validator " + wallet + " not found")
}

// SetStake is used to update the validator's stake when it changes
func (v *ValidatorsBook) SetStake(wallet string, stake uint32) error {
	for i, val := range v.valsArray {
		if val.wallet == wallet {
			v.totalstake += stake - val.stake
			v.valsArray[i].stake = stake
			return nil
		}
	}
	return errors.New("Validator " + wallet + " not found")
}

// ChooseValidator returns a validator's wallet, chosen randomly
// and proportionally to the stake
func (v *ValidatorsBook) ChooseValidator(seed int64) (string, error) {
	rand.Seed(seed)
	level := rand.Float64() * float64(v.totalstake)

	var counter uint32

	for _, val := range v.valsArray {
		counter += val.stake
		if float64(counter) >= level {
			return val.wallet, nil
		}
	}
	return "", errors.New("Validator could not be chosen")
}
