package networking

import (
	"fmt"
	"net/http"
	"time"

	"github.com/dexm-coin/dexmd/wallet"

	bp "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

// SendTransaction generates a transaction and broadcasts it
func SendTransaction(senderWallet *wallet.Wallet, recipient, fname string, amount, gas uint64, cdata []byte, ccreation bool, shard uint32) error {
	ips, err := GetPeerList("hackney")
	if err != nil {
		log.Error("peer ", err)
		return nil
	}

	dial := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 5 * time.Second,
	}

	log.Info(ips)
	for _, ip := range ips {
		conn, _, err := dial.Dial(fmt.Sprintf("ws://%s/ws", ip+":3141"), nil)
		if err != nil {
			log.Error(err)
			continue
		}

		req := &network.Request{
			Type: network.Request_GET_WALLET_STATUS,
		}

		reqD, _ := proto.Marshal(req)

		env := &network.Envelope{
			Type:  network.Envelope_REQUEST,
			Data:  reqD,
			Shard: uint32(senderWallet.Shard),
		}

		// GET_WALLET_STATUS requires to first send a request and then the address
		envD, _ := proto.Marshal(env)
		err = conn.WriteMessage(websocket.BinaryMessage, envD)
		if err != nil {
			log.Error(err)
			continue
		}

		senderAddr, err := senderWallet.GetWallet()
		if err != nil {
			log.Error(err)
		}
		senderEnv := &network.Envelope{
			Type:  network.Envelope_OTHER,
			Data:  []byte(senderAddr),
			Shard: uint32(senderWallet.Shard),
		}

		senderAddrD, _ := proto.Marshal(senderEnv)

		err = conn.WriteMessage(websocket.BinaryMessage, []byte(senderAddrD))
		if err != nil {
			log.Error(err)
			continue
		}

		walletEnv := &network.Envelope{}
		for {
			// Parse the message and save the new state
			_, msg, err := conn.ReadMessage()
			if err != nil {
				continue
			}

			err = proto.Unmarshal(msg, walletEnv)
			if err != nil {
				log.Error(err)
				continue
			}

			if walletEnv.Type == network.Envelope_OTHER {
				break
			}
			walletEnv = &network.Envelope{}
		}

		var walletStatus bp.AccountState
		err = proto.Unmarshal(walletEnv.Data, &walletStatus)
		if err != nil {
			log.Error(err)
		}
		log.Info("walletStatus ", walletStatus)
		senderWallet.Nonce = int(walletStatus.Nonce)
		senderWallet.Balance = int(walletStatus.Balance)

		trans, err := senderWallet.NewTransaction(recipient, amount, uint32(gas), cdata, shard)
		if err != nil {
			log.Error(err)
			continue
		}

		// signature := &network.Signature{
		// 	Pubkey: pub,
		// 	R:      r.Bytes(),
		// 	S:      s.Bytes(),
		// 	Data:   hash,
		// }
		trBroad := &network.Broadcast{
			Type: network.Broadcast_TRANSACTION,
			Data: trans,
			// identity
			TTL: 64,
		}
		brD, _ := proto.Marshal(trBroad)

		trEnv := &network.Envelope{
			Type:  network.Envelope_BROADCAST,
			Data:  brD,
			Shard: 0,
		}

		finalD, _ := proto.Marshal(trEnv)
		conn.WriteMessage(websocket.BinaryMessage, finalD)

		log.Info("Transaction done successfully")

	}

	return nil
}
