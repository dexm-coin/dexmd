package tests

import (
	"strconv"
	"testing"

	"github.com/dexm-coin/dexmd/blockchain"
)

func TestManageValidatorsBook(t *testing.T) {
	v := blockchain.NewValidatorsBook()
	// Test the addition of new validator
	v.AddValidator("deadbeef", 10)
	v.AddValidator("cafebabe", 100)
	v.AddValidator("cafebabe", 130)
	stake, err := v.GetStake("cafebabe")
	if err != nil {
		t.Error("Validator wasn't added correctly: ", err)
	}
	if stake != 130 {
		t.Error("stake is " + strconv.Itoa(int(stake)) + " while it should have been 130")
	}
	_, err = v.GetStake("stallman")
	if err == nil {
		t.Error("Found a non existent validator")
	}

	// Test the removal of validators
	v.AddValidator("stevejons", 42)
	v.AddValidator("timcrook", 1337)
	err = v.RemoveValidator("stallman")
	if err == nil {
		t.Error("Removed a nonexisting validator")
	}
	err = v.RemoveValidator("stevejons")
	if err != nil {
		t.Error(err)
	}
	_, err = v.GetStake("stevejons")
	if err == nil {
		t.Error("Validator wasn't removed correctly")
	}

	// Test the update of the stake
	stake, _ = v.GetStake("timcrook")
	if stake != 1337 {
		t.Error("Stake wasn't stored correctly")
	}
	err = v.SetStake("stallman", 3)
	if err == nil {
		t.Error("Updated the stake of a nonexisting validator")
	}
	err = v.SetStake("timcrook", 3)
	if err != nil {
		t.Error(err)
	}
	stake, _ = v.GetStake("timcrook")
	if stake != 1340 {
		t.Error("Stake wasn't updated correctly")
	}
}

func TestImportExportValidators(t *testing.T) {
	v1 := blockchain.NewValidatorsBook()
	validators := []string{"a", "b", "c", "d", "e", "f"}
	stakes := []uint64{1, 2, 3, 4, 5, 6}
	for i := 0; i < 6; i++ {
		v1.AddValidator(validators[i], stakes[i])
	}
	err := v1.ExportValidatorsBook("/tmp/validators")
	if err != nil {
		t.Error(err)
	}
	v2, err := blockchain.ImportValidatorsBook("/tmp/validators")
	if err != nil {
		t.Error(err)
	}
	if !v1.IsEqualTo(v2) {
		v1.Repr()
		v2.Repr()
		t.Error("Error during Import/Export")
	}
}

func TestChooseValidator(t *testing.T) {
	v := blockchain.NewValidatorsBook()
	validators := []string{"a", "b", "c", "d", "e", "f"}
	stakes := []uint64{1, 2, 3, 4, 5, 6}
	results := make(map[string]int)
	for i := 0; i < 6; i++ {
		v.AddValidator(validators[i], stakes[i])
		results[validators[i]] = 0
	}
	for i := 0; i < 21000; i++ {
		chosen, err := v.ChooseValidator(int64(i))
		if err != nil {
			t.Error(err)
		}
		results[chosen]++
	}
	for i := 0; i < 6; i++ {
		expected := int(stakes[i] * 1000)
		if results[validators[i]] < expected-50 || results[validators[i]] > expected+50 {
			t.Error("There is a weird fluctuation. Maybe it is nothing. Maybe not.")
		}
	}
}
