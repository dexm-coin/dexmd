package blockchain

import (
	"crypto/sha256"
	"errors"
	"math"
	"reflect"
	"strconv"
	"time"

	"github.com/dexm-coin/dexmd/util"
	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	pq "github.com/jupp0r/go-priority-queue"
	log "github.com/sirupsen/logrus"
	"github.com/syndtr/goleveldb/leveldb"
)

type MissingBlockStruct struct {
	sequenceBlock       uint64
	arrivalOrder        float64
	hightestBlock       string
	blocksToRecalculate []string
}

func (mb MissingBlockStruct) GetMBSequenceBlock() uint64 {
	return mb.sequenceBlock
}
func (mb MissingBlockStruct) GetMBArrivalOrder() float64 {
	return mb.arrivalOrder
}
func (mb MissingBlockStruct) GetMBHightestBlock() string {
	return mb.hightestBlock
}
func (mb MissingBlockStruct) GetMBbBlocksToRecalculate() []string {
	return mb.blocksToRecalculate
}

func (bc *Blockchain) ModifyMissingBlock(key string, mb MissingBlockStruct, hashCurrentBlock string) {
	bc.MissingBlock[key] = MissingBlockStruct{mb.sequenceBlock + 1, mb.arrivalOrder, mb.hightestBlock, append(mb.blocksToRecalculate, hashCurrentBlock)}
}
func (bc *Blockchain) AddMissingBlock(key string, arrivalOrder float64, hightestBlock string) {
	bc.MissingBlock[key] = MissingBlockStruct{1, arrivalOrder, hightestBlock, []string{}}
}

// Blockchain is an internal representation of a blockchain
type Blockchain struct {
	balancesDb    *leveldb.DB
	BlockDb       *leveldb.DB
	ContractDb    *leveldb.DB
	StateDb       *leveldb.DB
	CasperVotesDb *leveldb.DB

	Mempool *mempool

	Schnorr         map[string][]byte
	MTReceipt       [][]byte
	RSchnorr        [][]byte
	PSchnorr        [][]byte
	MessagesReceipt [][]byte

	GenesisTimestamp uint64

	CurrentBlock      uint64
	CurrentVote       uint64
	CurrentCheckpoint uint64

	CurrentValidator map[uint64]string

	PriorityBlocks pq.PriorityQueue
	MissingBlock   map[string]MissingBlockStruct
	HashBlocks     map[string]uint64
}

// BeaconChain is an internal representation of a beacon chain
type BeaconChain struct {
	MerkleRootsDb map[uint32]*leveldb.DB
	Validators    *ValidatorsBook

	CurrentBlock map[uint32]uint64
}

// NewBeaconChain create a new beacon chain
func NewBeaconChain(dbPath string) (*BeaconChain, error) {
	mrdb := make(map[uint32]*leveldb.DB)
	cb := make(map[uint32]uint64)

	for i := uint32(1); i < nShard+1; i++ {
		db, err := leveldb.OpenFile(dbPath+".merkleroots"+strconv.Itoa(int(i)), nil)
		if err != nil {
			return nil, err
		}
		mrdb[i] = db
		cb[i] = 0
	}

	vd := NewValidatorsBook()
	return &BeaconChain{
		MerkleRootsDb: mrdb,
		Validators:    vd,
		CurrentBlock:  cb,
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

	currentValidators := make(map[uint64]string)
	currentValidators[0] = "Dexm0135yvZqn8V7S88emfcJFzQMMMn3ARDCA241D2"
	currentValidators[1] = ""

	return &Blockchain{
		balancesDb:    db,
		BlockDb:       dbb,
		ContractDb:    cdb,
		StateDb:       sdb,
		CasperVotesDb: cvdb,

		Mempool: mp,

		Schnorr:         make(map[string][]byte),
		MTReceipt:       [][]byte{},
		RSchnorr:        [][]byte{},
		PSchnorr:        [][]byte{},
		MessagesReceipt: [][]byte{},

		CurrentBlock:      index,
		CurrentCheckpoint: 0,
		CurrentVote:       0,

		CurrentValidator: currentValidators,

		PriorityBlocks: pq.New(),
		MissingBlock:   make(map[string]MissingBlockStruct),
		HashBlocks:     make(map[string]uint64),
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

func (beaconChain *BeaconChain) SaveMerkleRoots(mr *protobufs.MerkleRootsSigned) error {
	res, _ := proto.Marshal(mr)
	currShard := mr.GetShard()
	log.Info("SaveMerkleRoots on shard ", currShard)

	beaconChain.CurrentBlock[currShard]++
	return beaconChain.MerkleRootsDb[currShard].Put([]byte(strconv.Itoa(int(beaconChain.CurrentBlock[currShard]))), res, nil)
}

func (beaconChain *BeaconChain) GetMerkleRoots(index uint64, shard uint32) ([]byte, error) {
	return beaconChain.MerkleRootsDb[shard].Get([]byte(strconv.Itoa(int(index))), nil)
}

// GetBlockBeacon returns the array of blocks at an index and at a specific shard
func (beaconChain *BeaconChain) GetBlockBeacon(index int64, shard uint32) ([]byte, error) {
	return beaconChain.MerkleRootsDb[shard].Get([]byte(strconv.Itoa(int(index))), nil)
}

// GetBlock returns the array of blocks at an index
func (bc *Blockchain) GetBlock(index uint64) ([]byte, error) {
	return bc.BlockDb.Get([]byte(strconv.Itoa(int(index))), nil)
}

// GetContractCode returns the code of a contract at an address. Used
// as a wrapper so when we add diffed contracts in the future it's easier
// to change without breaking everything
func (bc *Blockchain) GetContractCode(address []byte) ([]byte, error) {
	return bc.ContractDb.Get(address, nil)
}

// ValidateBlock checks the validity of a block. It uses the current
// blockchain state so the passed block might become valid in the future.
func (bc *Blockchain) ValidateBlock(block *protobufs.Block) (bool, error) {
	isTainted := make(map[string]bool)
	taintedState := make(map[string]protobufs.AccountState)

	// Genesis block is fine
	if block.GetIndex() == 0 {
		return true, nil
	}

	for i, t := range block.GetTransactions() {
		sender := wallet.BytesToAddress(t.GetSender(), t.GetShard())

		result, _ := proto.Marshal(t)
		bhash := sha256.Sum256(result)
		hash := bhash[:]
		valid, err := wallet.SignatureValid(t.GetSender(), t.GetR(), t.GetS(), hash)
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

		// Check if has already been send
		res, _ := proto.Marshal(t)
		dbKeyS := sha256.Sum256(res)
		dbKey := dbKeyS[:]
		data, err := bc.BlockDb.Get(dbKey, nil)
		if err == nil {
			tr := &protobufs.Transaction{}

			err = proto.Unmarshal(data, tr)
			if err != nil {
				return false, errors.New("Transaction was already included in db")
			}
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

	merkleRoot, err := GenerateMerkleTree(block.GetTransactions())
	if err != nil {
		log.Error(err)
		return false, err
	}
	if !reflect.DeepEqual(merkleRoot, block.GetMerkleRootReceipt()) {
		return false, errors.New("merkleRoot != block.GetMerkleRootReceipt())")
	}

	return true, nil
}

func (bc *Blockchain) SetState(wallet string, newState *protobufs.AccountState) error {
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
	sender := wallet.BytesToAddress(t.GetSender(), t.GetShard())

	if !wallet.IsWalletValid(t.GetRecipient()) {
		return errors.New("Invalid recipient")
	}

	// result, _ := proto.Marshal(t)
	// bhash := sha256.Sum256(result)
	// hash := bhash[:]
	// valid, err := wallet.SignatureValid(t.GetSender(), t.GetR(), t.GetS(), hash)
	// if !valid {
	// 	return err
	// }

	balance, err := bc.GetWalletState(sender)
	if err != nil {
		return err
	}

	// Check if balance is sufficient
	requiredBal, ok := util.AddU64O(t.GetAmount(), uint64(t.GetGas()))
	if requiredBal > balance.GetBalance() && ok {
		return errors.New("Balance is insufficient in transaction")
	}

	return nil
}

// GetNetworkIndex returns the current block index of the network
func (bc *Blockchain) GetNetworkIndex() int64 {
	timeSinceGenesis := time.Now().Unix() - int64(bc.GenesisTimestamp)

	index := math.Floor(float64(timeSinceGenesis) / 5.0)

	return int64(index)
}
