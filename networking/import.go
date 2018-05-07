package networking

import (
	"errors"
	"strconv"
	"time"

	"github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
)

// Maximum amount of blocks downloaded by one node at once
const (
	MAX_BLOCK_PEER = 200
)

// UpdateChain asks all connected nodes for their chain lenght, if any of them
// has a chain longer than the current one it will import
func (cs *ConnectionStore) UpdateChain() error {

	// Keep going till you have fully synced the chain
	for ok := true; ok; {
		didImport := false

		// Ask clients for the len of their chain
		for k := range cs.clients {
			req := &network.Request{
				Type: network.Request_GET_BLOCKCHAIN_LEN,
			}

			d, err := proto.Marshal(req)
			if err != nil {
				return err
			}

			env := &network.Envelope{
				Type: network.Envelope_REQUEST,
				Data: d,
			}

			d, err = proto.Marshal(env)
			if err != nil {
				return err
			}

			k.send <- d

			blockchainLen, err := k.GetResponse(500 * time.Millisecond)
			if err != nil {
				continue
			}

			flen, err := strconv.ParseUint(string(blockchainLen), 10, 64)
			if err != nil {
				continue
			}

			// We can import blocks from this node, start downloading and check them

			cb := cs.bc.CurrentBlock
			if flen > cb {

				// Limit the imported blocks per peer to MAX_BLOCKS_PEER
				for i := cb; cb < min(cb+MAX_BLOCK_PEER, flen); i++ {
					didImport = true
				}
			}
		}

		ok = didImport
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

func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
