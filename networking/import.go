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
func (cs *ConnectionStore) UpdateChain() error {
	for cs.bc.GetNetworkIndex() < 0 {
		log.Info("Waiting for genesis...")
		time.Sleep(1 * time.Second)
	}

	for cs.bc.CurrentBlock <= uint64(cs.bc.GetNetworkIndex()) {
		for k := range cs.clients {
			// Ask for blockchain len
			req := &network.Request{
				Type: network.Request_GET_BLOCKCHAIN_LEN,
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

			cb := cs.bc.CurrentBlock
			if flen < cb {
				continue
			}

			// Download new blocks up to MaxBlockPeer
			for i := cb; i < min(cb+MaxBlockPeer, flen); i++ {
				req = &network.Request{
					Type:  network.Request_GET_BLOCK,
					Index: i,
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

				index := &blockchain.Index{}
				err = proto.Unmarshal(block, index)
				if err != nil {
					break
				}

				for _, b := range index.GetBlocks() {
					res, err := cs.bc.ValidateBlock(b)
					if res {
						cs.bc.ImportBlock(b)
					} else {
						log.Error(err)
						break
					}

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
