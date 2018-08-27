package blockchain

import (
	"errors"
	"strconv"
	"time"

	"github.com/dexm-coin/dexmd/util"
	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
	"github.com/syndtr/goleveldb/leveldb"
)

// Blockchain is an internal representation of a blockchain
type Blockchain struct {
	balancesDb *leveldb.DB
	blockDb    *leveldb.DB
	Mempool    *mempool
	Validators *ValidatorsBook

	GenesisTimestamp  uint64
	CurrentBlock      uint64
	CurrentCheckpoint uint64
}

// NewBlockchain creates a database db
func NewBlockchain(dbPath string, index uint64) (*Blockchain, error) {
	db, err := leveldb.OpenFile(dbPath+".balances", nil)
	if err != nil {
		return nil, err
	}

	dbb, err := leveldb.OpenFile(dbPath+".blocks", nil)
	if err != nil {
		return nil, err
	}

	// 1MB blocks
	mp := newMempool(1000000, 100)

	vd, err := ImportValidatorsBook(dbPath + ".validators")
	if err != nil {
		return nil, err
	}

	return &Blockchain{
		balancesDb:        db,
		blockDb:           dbb,
		Mempool:           mp,
		Validators:        vd,
		CurrentBlock:      index,
		CurrentCheckpoint: 0,
	}, err
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

// SaveBlock saves an unvalidated block into the blockchain to be used with Casper
func (bc *Blockchain) SaveBlock(block *protobufs.Block) error {
	oldBlocks, err := bc.blockDb.Get([]byte(string(block.GetIndex())), nil)

	if block.GetIndex() == 0 {
		bc.GenesisTimestamp = block.GetTimestamp()
	}

	blocks := &protobufs.Index{}

	if err == nil {
		proto.Unmarshal(oldBlocks, blocks)
	}

	blocks.Blocks = append(blocks.Blocks, block)
	res, err := proto.Marshal(blocks)
	if err != nil {
		return err
	}

	return bc.blockDb.Put([]byte(string(block.GetIndex())), res, nil)
}

// GetBlocks returns the array of blocks at an index
func (bc *Blockchain) GetBlocks(index uint64) ([]byte, error) {
	return bc.blockDb.Get([]byte(string(index)), nil)
}

// ValidateBlock checks the validity of a block. It uses the current
// blockchain state so the passed block might become valid in the future.
// TODO Check validator
func (bc *Blockchain) ValidateBlock(block *protobufs.Block) (bool, error) {
	var isTainted map[string]bool
	var taintedState map[string]protobufs.AccountState

	// Genesis block is fine
	if block.GetIndex() == 0 {
		return true, nil
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

	if !wallet.IsWalletValid(t.GetRecipient()) {
		return errors.New("Invalid recipient")
	}

	valid, err := wallet.SignatureValid(t.GetSender(), t.GetR(), t.GetS(), []byte{})
	if !valid {
		return err
	}

	balance := protobufs.AccountState{}
	balance, err = bc.GetWalletState(sender)
	if err != nil {
		return err
	}

	if balance.Nonce != t.Nonce {
		return errors.New("Invalid nonce")
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
// This should be called on blocks that are finalized by PoS TODO Nonce for replays
func (bc *Blockchain) ImportBlock(block *protobufs.Block) error {
	res, err := bc.ValidateBlock(block)
	if !res {
		return err
	}

	// The genesis block is a title of a The Times article, We still need to
	// add a validator because otherwise no blocks will be generated
	if block.GetIndex() == 0 {
		// bc.Validators.AddValidator()
		return nil
	}

	totalGas := uint32(0)

	for _, t := range block.GetTransactions() {
		sender := wallet.BytesToAddress(t.GetSender())

		log.Info("Sender:", sender)
		log.Info("Recipient:", t.GetRecipient())
		log.Info("Amnt:", t.GetAmount())

		senderBalance, err := bc.GetWalletState(sender)
		if err != nil && block.GetIndex() != 0 {
			return err
		}

		if t.GetRecipient() == "DexmPoS" {
			bc.Validators.AddValidator(sender, t.GetAmount())
		}

		// Ignore error because if the wallet doesn't exist yet we don't care
		reciverBalance, _ := bc.GetWalletState(t.GetRecipient())

		// No overflow checks because ValidateBlock already does that
		senderBalance.Balance -= t.GetAmount() + uint64(t.GetGas())
		reciverBalance.Balance += t.GetAmount()

		// Avoid replaying transactions
		senderBalance.Nonce++

		log.Info("Sender balance:", senderBalance.Balance)
		log.Info("Reciver balance:", reciverBalance.Balance)

		totalGas += t.GetGas()

		bc.setState(sender, &senderBalance)
		bc.setState(t.GetRecipient(), &reciverBalance)
	}

	//bc.CurrentBlock++

	return nil
}

// GetNetworkIndex returns the current block index of the network
func (bc *Blockchain) GetNetworkIndex() int64 {
	return (time.Now().Unix() - int64(bc.GenesisTimestamp)) / 5
}
