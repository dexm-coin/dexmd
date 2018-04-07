package networking

import (
	"strconv"
	"time"

	"github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
)

var (
	bc       blockchain.Blockchain
	identity wallet.Wallet
)

func handleMessage(pb *protobufs.Request) []byte {
	switch pb.GetType() {
	// GET_BLOCKCHAIN_LEN returns the current block index
	case protobufs.Request_GET_BLOCKCHAIN_LEN:
		return []byte(strconv.FormatUint(bc.CurrentBlock, 10))

	// GET_IDENTITY returns the public key alongside with the current timestamp
	// signed by the key to prove ownership
	case protobufs.Request_GET_IDENTITY:
		currentTime := []byte(strconv.FormatInt(time.Now().UnixNano(), 10))

		// TODO Better error handling
		r, s, err := identity.Sign(currentTime)
		if err != nil {
			return []byte("ERROR")
		}

		toMarshal := &protobufs.Identity{}
		pub, err := identity.GetPubKey()
		if err != nil {
			return []byte("ERROR")
		}

		toMarshal.Pubkey = pub
		toMarshal.R = r.Bytes()
		toMarshal.S = s.Bytes()
		toMarshal.Data = currentTime

		encoded, err := proto.Marshal(toMarshal)
		if err != nil {
			return []byte("ERROR")
		}

		return encoded
	}

	return []byte{}
}
