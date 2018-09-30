package blockchain

import (
	"crypto/sha256"
	"errors"
	"reflect"
	"time"

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
func (bc *Blockchain) AddMempoolTransaction(rawTx []byte) error {
	pb := &protobufs.Transaction{}

	err := proto.Unmarshal(rawTx, pb)
	if err != nil {
		log.Error(err)
		return err
	}

	err = bc.ValidateTransaction(pb)
	if err != nil {
		log.Error(err)
		return err
	}

	priority := float64(pb.GetGas()) / float64(len(rawTx))

	if priority > bc.Mempool.maxGasPerByte {
		return errors.New("Too much gas")
	}

	bc.Mempool.queue.Insert(pb, priority)
	return nil
}

// GenerateBlock generates a valid unsigned block with transactions from the mempool
func (bc *Blockchain) GenerateBlock(miner string, shard uint32) (*protobufs.Block, error) {
	hash := []byte{}

	if bc.CurrentBlock != 0 {
		currBlocks := []byte{}
		var err error
		// There may be holes in the blockchain. Keep going till you find a block
		for i := bc.CurrentBlock - 1; i > 0; i-- {
			currBlocks, err = bc.GetBlock(i)
			if err == nil {
				break
			}
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
		tx, err := bc.Mempool.queue.Pop()

		// The mempool is empty, that's all the transactions we can include
		if err != nil {
			break
		}

		rtx := interface{}(tx).(*protobufs.Transaction)

		rawTx, err := proto.Marshal(rtx)
		if err != nil {
			break
		}

		// check if the hash of this transaction is inside bc.TransactionArrived that contain all the hash of the prev n transaction
		bhash := sha256.Sum256(rawTx)
		hash := bhash[:]
		alreadyReceived := false
		for _, h := range bc.TransactionArrived {
			equal := reflect.DeepEqual(h, hash)
			if equal {
				alreadyReceived = true
				break
			}
		}
		if alreadyReceived {
			continue
		}

		transactions = append(transactions, rtx)
		currentLen += len(rawTx)
	}

	block.Transactions = transactions

	merkleRootTransaction := []byte{}
	merkleRootReceipt := []byte{}
	if len(transactions) != 0 {
		merkleRootTransaction, merkleRootReceipt, err = GenerateMerkleTree(transactions)
		if err != nil {
			return nil, err
		}
	}

	block.MerkleRootTransaction = merkleRootTransaction
	block.MerkleRootReceipt = merkleRootReceipt

	return &block, nil
}
