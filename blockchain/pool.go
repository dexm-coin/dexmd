package blockchain

import (
	"crypto/sha256"
	"time"

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

	dbKeyS := sha256.Sum256(transaction)
	dbKey := dbKeyS[:]

	bc.BlockDb.Put(dbKey, transaction, nil)

	bc.Mempool.queue.Insert(&dbKey, float64(pb.GetGas()))
	return nil
}

// GenerateBlock generates a valid unsigned block with transactions from the mempool
func (bc *Blockchain) GenerateBlock(miner string, shard uint32, validators *ValidatorsBook) (*protobufs.Block, error) {
	hash := []byte{}

	if bc.CurrentBlock != 0 {
		currBlocks := []byte{}
		var err error
		// There may be holes in the blockchain. Keep going till you find a block
		for i := bc.CurrentBlock - 1; i >= 0; i-- {
			currBlocks, err = bc.GetBlock(i)
			if err == nil {
				break
			}
		}

		if err != nil {
			log.Error("Prev block not found")
		}

		bhash := sha256.Sum256(currBlocks)
		hash = bhash[:]
	}

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

		// check if the transazion is form my shard
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

		rawTx, err := proto.Marshal(rtx)
		if err != nil {
			break
		}

		// remove transaction from db as it was included
		_ = bc.BlockDb.Delete(*txKey, nil)

		// Don't include invalid transactions
		err = bc.ValidateTransaction(rtx)
		if err != nil {
			continue
		}

		transactions = append(transactions, rtx)
		currentLen += len(rawTx)
	}

	block.Transactions = transactions

	merkleRootReceipt := []byte{}
	if len(transactions) != 0 {
		merkleRootReceipt, err = GenerateMerkleTree(transactions)
		if err != nil {
			return nil, err
		}
	}

	block.MerkleRootReceipt = merkleRootReceipt

	return &block, nil
}
