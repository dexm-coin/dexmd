package blockchain

import (
	"errors"
	"strconv"

	"github.com/dexm-coin/dexmd/util"
	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	"github.com/syndtr/goleveldb/leveldb"
)

// Blockchain is an internal representation of a blockchain
type Blockchain struct {
	balancesDb *leveldb.DB
}

// NewBlockchain creates a database db
func NewBlockchain(dbPath string) (*Blockchain, error) {
	db, err := leveldb.OpenFile(dbPath, nil)

	return &Blockchain{db}, err
}

// GetWalletState returns the state of a wallet in the current block
func (bc *Blockchain) GetWalletState(wallet string) (protobufs.AccountState, error) {
	state := protobufs.AccountState{}
	raw, err := bc.balancesDb.Get([]byte(wallet), nil)
	if err != nil {
		return state, err
	}

	proto.Unmarshal(raw, &state)

	return state, nil
}

// ValidateBlock checks the validity of a block. It uses the current
// blockchain state so the passed block might become valid in the future.
func (bc *Blockchain) ValidateBlock(block *protobufs.Block) (bool, error) {
	var isTainted map[string]bool
	var taintedState map[string]protobufs.AccountState

	// Genesis block is fine
	if block.GetIndex() == 0 {
		return true, nil
	}

	var err error

	for i, t := range block.GetTransactions() {
		sender := wallet.BytesToAddress(t.GetSender())

		balance := protobufs.AccountState{}

		// Check if the address state changed while processing this block
		// If it hasn't changed then pull the state from the blockchain, otherwise
		// get the updated copy instead
		if !isTainted[sender] {
			balance, err = bc.GetWalletState(sender)
			if err != nil {
				return false, err
			}

		} else {
			balance = taintedState[sender]
		}

		// Check if balance is sufficient
		requiredBal, ok := util.AddU32O(t.GetAmount(), t.GetGas())
		if requiredBal > balance.GetBalance() && ok {
			return false, errors.New("Balance is insufficient in transaction " + strconv.Itoa(i))
		}

		// Check if nonce is correct
		newNonce, ok := util.AddU32O(balance.GetNonce(), 1)
		if t.GetNonce() != newNonce || !ok {
			return false, errors.New("Invalid nonce in transaction " + strconv.Itoa(i))
		}

		// Taint sender and update his balance. Reciver will be able to spend
		// his cash from the next block
		isTainted[sender] = true

		newBal, ok := util.SubU32O(balance.Balance, requiredBal)
		if !ok {
			return false, errors.New("Overflow in transaction " + strconv.Itoa(i))
		}

		balance.Balance = newBal
		taintedState[sender] = balance

		// To save a DB query we don't check the reciver for an overflow. If someone
		// gets that much cash we are gonna be fucked anyways because of PoS
	}

	return true, nil
}

// ImportBlock imports a block into the blockchain and checks if it's valid
// This should be called on blocks that are finalized by PoS
func (bc *Blockchain) ImportBlock(block *protobufs.Block) error {
	res, err := bc.ValidateBlock(block)
	if !res {
		return err
	}

	totalGas := uint32(0)

	for _, t := range block.GetTransactions() {
		senderBalance := protobufs.AccountState{}
		reciverBalance := protobufs.AccountState{}

		sender := wallet.BytesToAddress(t.GetSender())

		// Get state of reciver and sender
		rawSender, err := bc.balancesDb.Get([]byte(sender), nil)
		if err != nil {
			return err
		}

		proto.Unmarshal(rawSender, &senderBalance)

		rawReciver, err := bc.balancesDb.Get([]byte(t.GetRecipient()), nil)
		if err != nil {
			return err
		}

		proto.Unmarshal(rawReciver, &reciverBalance)

		// No overflow checks because ValidateBlock already does that
		senderBalance.Balance -= t.GetAmount() + t.GetGas()
		reciverBalance.Balance += t.GetAmount()

		totalGas += t.GetGas()
	}

	return nil
}
