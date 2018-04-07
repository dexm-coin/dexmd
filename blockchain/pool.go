package blockchain

import (
	"errors"
	"time"

	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	pq "github.com/jupp0r/go-priority-queue"
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
		return err
	}

	err = bc.ValidateTransaction(pb)
	if err != nil {
		return err
	}

	priority := float64(pb.GetGas()) / float64(len(rawTx))

	if priority > bc.mempool.maxGasPerByte {
		return errors.New("Too much gas")
	}

	bc.mempool.queue.Insert(pb, priority)
	return nil
}

// GenerateBlock generates a valid unsigned block with transactions from the mempool
func (bc *Blockchain) GenerateBlock(miner string) ([]byte, error) {
	block := protobufs.Block{
		Index:     bc.CurrentBlock + 1,
		Timestamp: uint64(time.Now().Unix()),
		Miner:     miner,
	}

	blockHeader, err := proto.Marshal(&block)
	if err != nil {
		return nil, err
	}

	var transactions []*protobufs.Transaction

	currentLen := len(blockHeader)

	// Check that the len is smaller than the max
	for currentLen < bc.mempool.maxBlockBytes {
		tx, err := bc.mempool.queue.Pop()

		// The mempool is empty, that's all the transactions we can include
		if err != nil {
			break
		}

		rtx := interface{}(tx).(*protobufs.Transaction)

		rawTx, err := proto.Marshal(rtx)
		if err != nil {
			break
		}

		transactions = append(transactions, rtx)
		currentLen += len(rawTx)
	}

	block.Transactions = transactions

	return proto.Marshal(&block)
}
