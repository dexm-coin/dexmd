package blockchain

import (
	"crypto/sha256"
	"errors"
	"time"

	"github.com/dexm-coin/dexmd/util"
	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	pq "github.com/jupp0r/go-priority-queue"
	log "github.com/sirupsen/logrus"
)

type mempool struct {
	maxBlockBytes int
	maxGasPerByte float64
	queue         pq.PriorityQueue
}

func newMempool(maxBlockSize int, maxGas float64) *mempool {
	return &mempool{maxBlockSize, maxGas, pq.New()}
}

// AddMempoolTransaction adds a transaction to the mempool
// TODO Validate gas
func (bc *Blockchain) AddMempoolTransaction(pb *protobufs.Transaction, transaction []byte) error {
	err := bc.ValidateTransaction(pb)
	if err != nil {
		log.Error(err)
		return err
	}

	// save the hash of the transaction and put it on blockdb
	dbKeyS := sha256.Sum256(transaction)
	dbKey := dbKeyS[:]
	_, err = bc.BlockDb.Get(dbKey, nil)
	if err == nil {
		return errors.New("Already in db")
	}

	bc.BlockDb.Put(dbKey, transaction, nil)

	bc.Mempool.queue.Insert(&dbKey, float64(pb.GetGas()))
	return nil
}

// GenerateBlock generates a valid unsigned block with transactions from the mempool
func (bc *Blockchain) GenerateBlock(miner string, shard uint32, validators *ValidatorsBook) (*protobufs.Block, error) {
	
	hash := []byte{}
	hashInterface, err := bc.PriorityBlocks.Pop()
	if err != nil {
		
	}
	hashString := interface{}(hashInterface).(*string)
	hash = []byte(*hashString)

	block := protobufs.Block{
		Index:     bc.CurrentBlock,
		Timestamp: uint64(time.Now().Unix()),
		Miner:     miner,
		PrevHash:  hash,
		Shard:     shard,
	}

	blockHeader, err := proto.Marshal(&block)
	if err != nil {
		return nil, err
	}

	var transactions []*protobufs.Transaction

	currentLen := len(blockHeader)
	isTainted := make(map[string]bool)
	taintedState := make(map[string]protobufs.AccountState)

	// Check that the len is smaller than the max
	for currentLen < bc.Mempool.maxBlockBytes {
		txB, err := bc.Mempool.queue.Pop()

		// The mempool is empty, that's all the transactions we can include
		if err != nil {
			break
		}

		txKey := interface{}(txB).(*[]byte)
		txData, err := bc.BlockDb.Get(*txKey, nil)
		if err != nil {
			continue
		}

		rtx := &protobufs.Transaction{}
		proto.Unmarshal(txData, rtx)

		// Don't include invalid transactions
		err = bc.ValidateTransaction(rtx)
		if err != nil {
			continue
		}

		// check if the transazion is for my shard
		senderWallet := wallet.BytesToAddress(rtx.GetSender(), rtx.GetShard())
		shardSender, err := validators.GetShard(senderWallet)
		if err != nil {
			log.Error(err)
			continue
		}
		currentShard, err := validators.GetShard(miner)
		if err != nil {
			log.Error(err)
			continue
		}
		if shardSender != currentShard {
			continue
		}

		balance := protobufs.AccountState{}

		// Check if the address state changed while processing this block
		// If it hasn't changed then pull the state from the blockchain, otherwise
		// get the updated copy instead
		if !isTainted[senderWallet] {
			balance, err = bc.GetWalletState(senderWallet)
			if err != nil {
				continue
			}
		} else {
			balance = taintedState[senderWallet]
		}

		// Check if balance is sufficient
		requiredBal, ok := util.AddU64O(rtx.GetAmount(), uint64(rtx.GetGas()))
		if requiredBal > balance.GetBalance() && ok {
			continue
		}

		newBal, ok := util.SubU64O(balance.Balance, requiredBal)
		if !ok {
			continue
		}

		balance.Balance = newBal
		isTainted[senderWallet] = true
		taintedState[senderWallet] = balance

		rawTx, err := proto.Marshal(rtx)
		if err != nil {
			break
		}

		transactions = append(transactions, rtx)
		currentLen += len(rawTx)
	}

	block.Transactions = transactions

	// create the merkletree with the transactions and get its root
	merkleRootReceipt := []byte{}
	if len(transactions) != 0 {
		merkleRootReceipt, err = GenerateMerkleTree(transactions)
		if err != nil {
			return nil, err
		}
	}

	// put the merkleroot inside the block
	block.MerkleRootReceipt = merkleRootReceipt

	return &block, nil
}
