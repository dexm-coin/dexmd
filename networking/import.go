package networking

import (
	"errors"
	"strconv"
	"time"

	"github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
)

// Maximum amount of blocks downloaded by one node at once
const (
	MaxBlockPeer = 200
)

// UpdateChain asks all connected nodes for their chain lenght, if any of them
// has a chain longer than the current one it will import
// TODO Drop client if err != nil
func (cs *ConnectionStore) UpdateChain(nextShard int64) error {
	// No need to sync before genesis
	for cs.shardChain.GetNetworkIndex() < 0 {
		return nil
	}

	for cs.shardChain.CurrentBlock <= uint64(cs.shardChain.GetNetworkIndex()) {
		for k := range cs.clients {
			// Ask for blockchain len
			req := &network.Request{
				Type:  network.Request_GET_BLOCKCHAIN_LEN,
				Shard: nextShard,
			}

			d, err := makeReqEnvelope(req)
			if err != nil {
				continue
			}

			k.send <- d

			blockchainLen, err := k.GetResponse(300 * time.Millisecond)
			if err != nil {
				continue
			}

			flen, err := strconv.ParseUint(string(blockchainLen), 10, 64)
			if err != nil {
				continue
			}

			cb := cs.shardChain.CurrentBlock
			if flen < cb {
				continue
			}

			// Download new blocks up to MaxBlockPeer
			for i := cb; i < min(cb+MaxBlockPeer, flen); i++ {
				req = &network.Request{
					Type:  network.Request_GET_BLOCK,
					Index: i,
					Shard: nextShard,
				}

				d, err = makeReqEnvelope(req)
				if err != nil {
					break
				}

				k.send <- d

				block, err := k.GetResponse(300 * time.Millisecond)
				if err != nil {
					break
				}

				b := &blockchain.Block{}
				err = proto.Unmarshal(block, b)
				if err != nil {
					break
				}

				res, err := cs.shardChain.ValidateBlock(b)
				if res {
					cs.shardChain.ImportBlock(b)
					cs.shardChain.CurrentBlock++
				} else {
					log.Error("import ", err)
				}
			}
		}
	}

	return nil
}

func (c *client) GetResponse(timeout time.Duration) ([]byte, error) {
	select {
	case res := <-c.readOther:
		return res, nil
	case <-time.After(timeout):
		return nil, errors.New("Response timed out")
	}
}

func makeReqEnvelope(req *network.Request) ([]byte, error) {
	d, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}

	env := &network.Envelope{
		Type: network.Envelope_REQUEST,
		Data: d,
	}

	d, err = proto.Marshal(env)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
