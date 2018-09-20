package blockchain

import (
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/dexm-coin/dexmd/contracts"
	"github.com/dexm-coin/dexmd/util"
	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
	"github.com/syndtr/goleveldb/leveldb"
)

// Blockchain is an internal representation of a blockchain
type Blockchain struct {
	balancesDb    *leveldb.DB
	blockDb       *leveldb.DB
	ContractDb    *leveldb.DB
	StateDb       *leveldb.DB
	CasperVotesDb *leveldb.DB

	Mempool *mempool
	// Validators *ValidatorsBook

	GenesisTimestamp uint64

	CurrentBlock            uint64
	CurrentCheckpoint       uint64
	CurrentValidator        string
	CurrentVote             uint64
	CurrentMerkleRootSigned []byte
}

// BeaconChain is an internal representation of a beacon chain
type BeaconChain struct {
	MerkleRootsDb map[int64]*leveldb.DB
	Validators    *ValidatorsBook

	CurrentBlock map[int64]uint64
	CurrentSign  map[int64]int64
}

// NewBeaconChain create a new beacon chain
func NewBeaconChain(dbPath string) (*BeaconChain, error) {
	mrdb := make(map[int64]*leveldb.DB)
	cb := make(map[int64]uint64)
	cs := make(map[int64]int64)

	for i := 1; i < 101; i++ {
		db, err := leveldb.OpenFile(dbPath+".merkleroots"+strconv.Itoa(i), nil)
		if err != nil {
			return nil, err
		}
		mrdb[int64(i)] = db
		cb[int64(i)] = 0
		cs[int64(i)] = 0
	}

	vd := NewValidatorsBook()
	return &BeaconChain{
		MerkleRootsDb: mrdb,
		Validators:    vd,
		CurrentBlock:  cb,
		CurrentSign:   cs,
	}, nil
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

	cdb, err := leveldb.OpenFile(dbPath+".code", nil)
	if err != nil {
		return nil, err
	}

	sdb, err := leveldb.OpenFile(dbPath+".memory", nil)
	if err != nil {
		return nil, err
	}

	cvdb, err := leveldb.OpenFile(dbPath+".votes", nil)
	if err != nil {
		return nil, err
	}

	// 1MB blocks
	mp := newMempool(1000000, 100)

	// vd := NewValidatorsBook()

	return &Blockchain{
		balancesDb:    db,
		blockDb:       dbb,
		ContractDb:    cdb,
		StateDb:       sdb,
		CasperVotesDb: cvdb,

		Mempool: mp,
		// Validators: vd,

		CurrentBlock:      index,
		CurrentCheckpoint: 0,
		CurrentVote:       0,
		CurrentMerkleRootSigned: []byte{},
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

// SaveBlockBeacon saves a block into the BeaconChain in a specific shard and index
func (bc *BeaconChain) SaveBlockBeacon(block *protobufs.MerkleRoot, shard, index int64) error {
	res, _ := proto.Marshal(block)
	return bc.MerkleRootsDb[shard].Put([]byte(strconv.Itoa(int(index))), res, nil)
}

// GetBlockBeacon returns the array of blocks at an index and at a specific shard
func (bc *BeaconChain) GetBlockBeacon(index, shard int64) ([]byte, error) {
	return bc.MerkleRootsDb[shard].Get([]byte(strconv.Itoa(int(index))), nil)
}

// SaveBlock saves an unvalidated block into the blockchain to be used with Casper
func (bc *Blockchain) SaveBlock(block *protobufs.Block) error {
	res, _ := proto.Marshal(block)
	return bc.blockDb.Put([]byte(strconv.Itoa(int(block.GetIndex()))), res, nil)
}

// GetBlock returns the array of blocks at an index
func (bc *Blockchain) GetBlock(index uint64) ([]byte, error) {
	return bc.blockDb.Get([]byte(strconv.Itoa(int(index))), nil)
}

// ValidateBlock checks the validity of a block. It uses the current
// blockchain state so the passed block might become valid in the future.
// TODO Check validator
func (bc *Blockchain) ValidateBlock(block *protobufs.Block) (bool, error) {
	isTainted := make(map[string]bool)
	taintedState := make(map[string]protobufs.AccountState)

	// Genesis block is fine
	if block.GetIndex() == 0 {
		return true, nil
	}

	for i, t := range block.GetTransactions() {
		sender := wallet.BytesToAddress(t.GetSender())

		result, _ := proto.Marshal(t)

		valid, err := wallet.SignatureValid(t.GetSender(), t.GetR(), t.GetS(), result)
		if !valid || err != nil {
			log.Error("SignatureValid ", err)
			return false, err
		}

		balance := protobufs.AccountState{}

		// Check if the address state changed while processing this block
		// If it hasn't changed then pull the state from the blockchain, otherwise
		// get the updated copy instead
		if !isTainted[sender] {
			balance, err = bc.GetWalletState(sender)
			if err != nil {
				log.Error("getwalletstate ", err)
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
		newNonce, ok := util.AddU32O(balance.GetNonce(), uint32(1))

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
		balance.Nonce++
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

	// TODO change []byte{} with the hash of the transaction
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
	if t.GetNonce() != balance.GetNonce() || !ok {
		return errors.New("Invalid nonce in transaction")
	}

	return nil
}

// ImportBlock imports a block into the blockchain and checks if it's valid
// This should be called on blocks that are finalized by PoS
func (bc *Blockchain) ImportBlock(block *protobufs.Block) error {
	res, err := bc.ValidateBlock(block)
	if !res {
		log.Error("ImportBlock ", err)
		return err
	}

	// The genesis block is a title of a The Times article, We still need to
	// add a validator because otherwise no blocks will be generated
	if block.GetIndex() == 0 {
		// A bit of balance to run tests
		state := &protobufs.AccountState{
			Balance: 20000,
			Nonce:   0,
		}

		bc.setState("Dexm3ENiLVMNwaeRswEbV1PT7UEpDNwwlbef2e683", state)
		bc.Validators.AddValidator("Dexm3ENiLVMNwaeRswEbV1PT7UEpDNwwlbef2e683", 20000, -300)

		state = &protobufs.AccountState{
			Balance: 10000,
			Nonce:   0,
		}

		bc.setState("Dexm25g6YbMNWpu9LHqCTP7S8r2PHBMHla441f087", state)
		bc.Validators.AddValidator("Dexm25g6YbMNWpu9LHqCTP7S8r2PHBMHla441f087", 10000, -300)

		return nil
	}

	totalGas := uint32(0)

	for _, t := range block.GetTransactions() {
		sender := wallet.BytesToAddress(t.GetSender())

		log.Info("Sender:", sender)
		log.Info("Recipient:", t.GetRecipient())
		log.Info("Amnt:", t.GetAmount())

		senderBalance, err := bc.GetWalletState(sender)
		if err != nil {
			log.Error(err)
			return err
		}

		if t.GetRecipient() == "DexmPoS" {
			exist := bc.Validators.AddValidator(sender, t.GetAmount(), int64(bc.CurrentBlock))
			if exist {
				log.Info("slash for ", sender)
			}
		}

		// Ignore error because if the wallet doesn't exist yet we don't care
		reciverBalance, _ := bc.GetWalletState(t.GetRecipient())

		// No overflow checks because ValidateBlock already does that
		senderBalance.Balance -= t.GetAmount() + uint64(t.GetGas())
		reciverBalance.Balance += t.GetAmount()

		// Avoid replaying transactions
		senderBalance.Nonce++

		log.Info("Sender balance:", senderBalance.Balance)
		log.Info("Sender nonce: ", senderBalance.Nonce)
		log.Info("Reciver balance:", reciverBalance.Balance)

		totalGas += t.GetGas()

		err = bc.setState(sender, &senderBalance)
		if err != nil {
			log.Error(err)
			return err
		}
		err = bc.setState(t.GetRecipient(), &reciverBalance)
		if err != nil {
			log.Error(err)
			return err
		}

		if t.GetContractCreation() {
			// Use the code, sender and prev hash to decide contract address
			contractAddrSource := append(t.GetData(), t.GetSender()...)
			contractAddrSource = append(contractAddrSource, block.PrevHash...)
			contractAddr := wallet.BytesToAddress(contractAddrSource)

			// Save it on a separate db
			log.Info("New contract at ", contractAddr)
			bc.ContractDb.Put([]byte(contractAddr), t.GetData(), nil)
		}

		// If a function identifier is specified then fetch the contract and execute
		if t.GetFunction() != "" {
			c, err := contracts.GetContract(t.GetRecipient(), bc.ContractDb, bc.StateDb)
			if err != nil {
				return err
			}

			err = c.ExecuteContract(t.GetFunction(), t.GetArgs())
			if err != nil {
				return err
			}

			c.SaveState()
		}
	}

	return nil
}

// GetNetworkIndex returns the current block index of the network
func (bc *Blockchain) GetNetworkIndex() int64 {
	timeSinceGenesis := time.Now().Unix() - int64(bc.GenesisTimestamp)

	index := math.Floor(float64(timeSinceGenesis) / 5.0)

	return int64(index)
}
