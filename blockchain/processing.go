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
	balancesDb   *leveldb.DB
	mempool      *mempool
	CurrentBlock uint64
}

// NewBlockchain creates a database db
func NewBlockchain(dbPath string, blocks uint64) (*Blockchain, error) {
	db, err := leveldb.OpenFile(dbPath, nil)

	// 1MB blocks
	mp := newMempool(1000000, 100)

	return &Blockchain{db, mp, blocks}, err
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

	// Check that we haven't passed that block already
	if block.GetIndex() < bc.CurrentBlock {
		return false, errors.New("Block index is too small")
	}

	for i, t := range block.GetTransactions() {
		sender := wallet.BytesToAddress(t.GetSender())

		valid, err := wallet.SignatureValid(t.GetSender(), t.GetR(), t.GetS(), []byte{})
		if !valid {
			return false, err
		}

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
		requiredBal, ok := util.AddU64O(t.GetAmount(), uint64(t.GetGas()))
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

		newBal, ok := util.SubU64O(balance.Balance, requiredBal)
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

func (bc *Blockchain) setState(wallet string, newState *protobufs.AccountState) error {
	stateBytes, err := proto.Marshal(newState)
	if err != nil {
		return err
	}

	return bc.balancesDb.Put([]byte(wallet), stateBytes, nil)
}

// ValidateTransaction validates a transaction with the current state.
// Different from ValidateBlock because that has to verify for double spends
// inside the same block.
func (bc *Blockchain) ValidateTransaction(t *protobufs.Transaction) error {
	sender := wallet.BytesToAddress(t.GetSender())

	valid, err := wallet.SignatureValid(t.GetSender(), t.GetR(), t.GetS(), []byte{})
	if !valid {
		return err
	}

	balance := protobufs.AccountState{}
	balance, err = bc.GetWalletState(sender)
	if err != nil {
		return err
	}

	// Check if balance is sufficient
	requiredBal, ok := util.AddU64O(t.GetAmount(), uint64(t.GetGas()))
	if requiredBal > balance.GetBalance() && ok {
		return errors.New("Balance is insufficient in transaction")
	}

	// Check if nonce is correct
	newNonce, ok := util.AddU32O(balance.GetNonce(), 1)
	if t.GetNonce() != newNonce || !ok {
		return errors.New("Invalid nonce in transaction")
	}

	return nil
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
		sender := wallet.BytesToAddress(t.GetSender())

		senderBalance, err := bc.GetWalletState(sender)
		if err != nil {
			return err
		}

		// Ignore error because if the wallet doesn't exist yet we don't care
		reciverBalance, _ := bc.GetWalletState(t.GetRecipient())

		// No overflow checks because ValidateBlock already does that
		senderBalance.Balance -= t.GetAmount() + uint64(t.GetGas())
		reciverBalance.Balance += t.GetAmount()

		totalGas += t.GetGas()

		bc.setState(sender, &senderBalance)
		bc.setState(t.GetRecipient(), &reciverBalance)
	}

	bc.CurrentBlock++

	return nil
}
