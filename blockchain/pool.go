package blockchain

import (
	"crypto/sha256"
	"errors"
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
func (bc *Blockchain) GenerateBlock(miner string) (*protobufs.Block, error) {
	hash := []byte{}

	if bc.CurrentBlock != 0 {
		// There may be holes in the blockchain. Keep going till you find a block
		// for i := bc.CurrentBlock - 1; err != nil; i-- {
		// 	currBlocks, err = bc.GetBlocks(i)
		// }

		currBlocks, err := bc.GetBlocks(bc.CurrentBlock - 1)
		if err != nil {
			log.Error("leveldb ", err)
		}

		index := &protobufs.Index{}
		proto.Unmarshal(currBlocks, index)
		if len(index.GetBlocks()) != 0 {
			selectedBlock := index.GetBlocks()[0]

			blockBytes, err := proto.Marshal(selectedBlock)
			if err != nil {
				return nil, err
			}

			bhash := sha256.Sum256(blockBytes)
			hash = bhash[:]
		}
	}

	block := protobufs.Block{
		Index:     bc.CurrentBlock, // was +1
		Timestamp: uint64(time.Now().Unix()),
		Miner:     miner,
		PrevHash:  hash,
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

		transactions = append(transactions, rtx)
		currentLen += len(rawTx)
	}

	block.Transactions = transactions

	return &block, nil
}
