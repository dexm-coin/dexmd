package networking

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"strconv"

	protobufs "github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
)

func (cs *ConnectionStore) handleMessage(dataEnvelope []byte, c *client, shard uint32, identity *protobufs.Signature) []byte {
	request := &protobufs.Request{}
	err := proto.Unmarshal(dataEnvelope, request)
	if err != nil {
		log.Error(err)
		return []byte("Error")
	}
	dataMessage := request.GetData()

	if !reflect.DeepEqual(request.GetAddress(), identity.GetPubkey()) {
		log.Error("broadcast.GetAddress() != identity.GetPubkey()")
		return []byte("Error")
	}

	switch request.GetType() {
	// GET_BLOCKCHAIN_LEN returns the current block index
	case protobufs.Request_GET_BLOCKCHAIN_LEN:
		if !cs.CheckShard(shard) {
			return []byte("Error")
		}
		return []byte(strconv.FormatUint(cs.shardsChain[shard].CurrentBlock, 10))

	// GET_PEERS returns the peers the node is currently connected to
	case protobufs.Request_GET_PEERS:
		peers := []string{}

		for k := range cs.clients {
			peers = append(peers, k.conn.RemoteAddr().String())
		}

		resp := &protobufs.Peers{
			Ip: peers,
		}

		b, err := proto.Marshal(resp)
		if err != nil {
			return []byte("Error")
		}

		return b

	// GET_BLOCK returns a block at the passed index and shard
	case protobufs.Request_GET_BLOCK:
		if !cs.CheckShard(shard) {
			return []byte("Error")
		}

		var index uint64
		buff := bytes.NewReader(dataMessage)
		err := binary.Read(buff, binary.LittleEndian, &index)
		if err != nil {
			log.Error("binary.Read failed:", err)
			return []byte("Error")
		}

		block, err := cs.shardsChain[shard].GetBlock(index)
		if err != nil {
			return []byte("Error")
		}
		return block

	// GET_WALLET_STATUS returns the current balance and nonce of a wallet
	case protobufs.Request_GET_WALLET_STATUS:
		if !cs.CheckShard(shard) {
			return []byte("Error")
		}

		state, err := cs.shardsChain[shard].GetWalletState(fmt.Sprintf("%s", dataMessage))
		if err != nil {
			return []byte("Error")
		}

		data, err := proto.Marshal(&state)
		if err != nil {
			return []byte("Error")
		}
		return data

	// GET_VERSION returns the version of the node
	case protobufs.Request_GET_VERSION:
		return []byte("0.0 Hackney")

	// GET_CONTRACT_CODE returns the code of a contract
	case protobufs.Request_GET_CONTRACT_CODE:
		if !cs.CheckShard(shard) {
			return []byte("Error")
		}

		code, err := cs.shardsChain[shard].GetContractCode(dataMessage)
		if err != nil {
			return []byte("Error")
		}

		return code

	// GET_INTERESTS returns the type of broadcasts the client is interested in
	case protobufs.Request_GET_INTERESTS:
		keys := []string{}

		for k := range cs.interests {
			keys = append(keys, k)
		}

		p := &protobufs.Interests{
			Keys: keys,
		}
		log.Info("Request_GET_INTERESTS ", p)

		d, _ := proto.Marshal(p)
		return d

		// case protobufs.Request_HASH_EXIST:
		// 	// rivece un indice di un blocco, e deve ritornare l'hash di quel blocco
		// 	// TODOMaybe

		// case protobufs.Request_GET_WALLET:
		// 	// riceve un messaggio casuale, che deve firmare, e deve anche mandare la sua chiave pubblica per poter decifrare
		// 	// TODOMaybe cambia il messaggio con un ts e controlla che sia valido
		// 	randomMessage := dataMessage.GetData()

		// 	msg := &protobufs.RandomMessage{
		// 		Pubkey: cs.identity.GetPubKey(),
		// 		Data:   randomMessage,
		// 	}
		// 	byteMsg, _ := proto.Marshal(msg)

		// 	r, s, err := cs.identity.Sign(byteMsg)
		// 	if err != nil {
		// 		log.Error(err)
		// 		return []byte{"Error"}
		// 	}

		// 	signature := &protobufs.Signature{
		// 		PubKey: cs.identity.GetPubKey(),
		// 		R:      r,
		// 		S:      s,
		// 		Data:   byteMsg,
		// 		Shard:  shard,
		// 	}
		// 	d, _ := proto.Marshal(signature)
		// 	return d

	}

	return []byte{}
}
