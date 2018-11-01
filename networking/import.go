package networking

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/wallet"
	"github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"

	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	log "github.com/sirupsen/logrus"
)

// Maximum amount of blocks downloaded by one node at once
const (
	MaxBlockPeer = 200
)

// UpdateChain asks all connected nodes for their chain lenght, if any of them
// has a chain longer than the current one it will import
// TODO Drop client if err != nil
// TODO Make everything again
func (cs *ConnectionStore) UpdateChain(nextShard uint32) error {
	// No need to sync before genesis
	for cs.shardsChain[nextShard].GetNetworkIndex() < 0 {
		return nil
	}

	/*for cs.shardsChain[nextShard].CurrentBlock <= uint64(cs.shardsChain[nextShard].GetNetworkIndex()) {
		for k := range cs.clients {
			// Ask for blockchain len
			req := &network.Request{
				Type: network.Request_GET_BLOCKCHAIN_LEN,
			}

			wallet, err := cs.identity.GetWallet()
			if err != nil {
				log.Error(err)
				continue
			}
			currentShard, err := cs.beaconChain.Validators.GetShard(wallet)

			d, err := makeReqEnvelope(req, currentShard)
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

			cb := cs.shardsChain[nextShard].CurrentBlock
			if flen < cb {
				continue
			}

			// Download new blocks up to MaxBlockPeer
			for i := cb; i < min(cb+MaxBlockPeer, flen); i++ {
				byteI := make([]byte, 4)
				binary.LittleEndian.PutUint64(byteI, i)

				env := &network.Envelope{
					Type:  network.Envelope_REQUEST,
					Data:  byteI,
					Shard: nextShard,
				}

				req := &network.Request{
					Type: network.Request_GET_BLOCK,
					Data: env,
				}
				reqD, _ := proto.Marshal(req)

				k.send <- reqD

				block, err := k.GetResponse(300 * time.Millisecond)
				if err != nil {
					break
				}

				b := &protobufs.Block{}
				err = proto.Unmarshal(block, b)
				if err != nil {
					break
				}

				res, err := cs.shardsChain[nextShard].ValidateBlock(b)
				if res && err == nil {
					cs.ImportBlock(b, uint32(nextShard))
					cs.shardsChain[nextShard].CurrentBlock++
				} else {
					log.Error("import ", err)
				}
			}
		}
	}*/
	return nil
}

func MakeSpecificRequest(w *wallet.Wallet, shard uint32, dataRequest []byte, t network.Request_MessageTypes, conn *websocket.Conn, shardAddress uint32) error {
	pubKey, _ := w.GetPubKey()

	req := &network.Request{
		Type:         t,
		Data:         dataRequest,
		Address:      pubKey,
		ShardAddress: shardAddress,
	}

	requestBytes, _ := proto.Marshal(req)
	bhashRequest := sha256.Sum256(requestBytes)
	hashRequest := bhashRequest[:]

	// Sign the new block
	r, s, err := w.Sign(hashRequest)
	if err != nil {
		log.Error(err)
	}

	signature := &network.Signature{
		Pubkey: pubKey,
		R:      r.Bytes(),
		S:      s.Bytes(),
		Data:   hashRequest,
	}

	env := &network.Envelope{
		Data:     requestBytes,
		Type:     network.Envelope_BROADCAST,
		Shard:    shard,
		Identity: signature,
		TTL:      64,
	}
	data, _ := proto.Marshal(env)

	err = conn.WriteMessage(websocket.BinaryMessage, data)
	if err != nil {
		log.Error(err)
		return err
	}
	return nil
}

func (cs *ConnectionStore) MakeEnvelopeBroadcast(dataBroadcast []byte, typeBroadcast network.Broadcast_BroadcastType, shardAddress uint32, shardEnvelope uint32) []byte {
	pubKey, _ := cs.identity.GetPubKey()

	// Create a broadcast message and send it to the network
	broadcast := &network.Broadcast{
		Data:         dataBroadcast,
		Type:         network.Broadcast_BLOCK_PROPOSAL,
		Address:      pubKey,
		ShardAddress: shardAddress,
	}
	broadcastBytes, _ := proto.Marshal(broadcast)
	bhashBroadcast := sha256.Sum256(broadcastBytes)
	hashBroadcast := bhashBroadcast[:]

	// Sign the new block
	r, s, err := cs.identity.Sign(hashBroadcast)
	if err != nil {
		log.Error(err)
	}

	signature := &network.Signature{
		Pubkey: pubKey,
		R:      r.Bytes(),
		S:      s.Bytes(),
		Data:   hashBroadcast,
	}

	env := &network.Envelope{
		Data:     broadcastBytes,
		Type:     network.Envelope_BROADCAST,
		Shard:    shardEnvelope,
		Identity: signature,
		TTL:      64,
	}

	data, _ := proto.Marshal(env)
	return data
}

// func (cs *ConnectionStore) RequestHashBlock(shard uint32, indexBlock uint64, hashBlock []byte) (*protobufs.Block, bool) {

// 	// TODO ASAP non chiedo a tutti i client ma solo ai validator nella mia shard
// 	for c := range cs.clients {
// 		byteI := make([]byte, 4)
// 		binary.LittleEndian.PutUint64(byteI, indexBlock)

// 		err := MakeSpecificRequest(shard, byteI, network.Request_HASH_EXIST, c.conn)
// 		if err != nil {
// 			log.Error(err)
// 			continue
// 		}
// 		hashBlockReceived, err := c.GetResponse(100 * time.Millisecond)
// 		if err != nil {
// 			log.Error(err)
// 			continue
// 		}

// 		equal := reflect.DeepEqual(hashBlock, hashBlockReceived)
// 		if equal {
// 			// TODO ASAP in base allo stake conta quanti ti hanno dato il blocco e quello giusto
// 		}
// 	}
// 	return nil, false
// }

// func (cs *ConnectionStore) RequestMissingBlock(shard uint32, indexBlock uint64) (*protobufs.Block, bool) {

// 	// TODO ASAP non chiedo a tutti i client ma solo ai validator nella mia shard
// 	for c := range cs.clients {
// 		byteI := make([]byte, 4)
// 		binary.LittleEndian.PutUint64(byteI, indexBlock)

// 		err := MakeSpecificRequest(shard, byteI, network.Request_GET_BLOCK, c.conn)
// 		if err != nil {
// 			log.Error(err)
// 			continue
// 		}

// 		// TODO the request of the block can be substitue with zk snark that give you the proof that this block exist
// 		// without recive the whole block
// 		byteBlock, err := c.GetResponse(200 * time.Millisecond)
// 		if err != nil {
// 			log.Error(err)
// 			continue
// 		}
// 		block := &protobufs.Block{}
// 		err = proto.Unmarshal(byteBlock, block)
// 		if err != nil {
// 			log.Error("error on Unmarshal")
// 			continue
// 		}

// 		verify, err := cs.shardsChain[shard].ValidateBlock(block)
// 		if verify {
// 			// TODO ASAP in base allo stake conta quanti ti hanno dato il blocco e quello giusto
// 		}
// 	}

// 	return nil, false
// }

// SaveBlock saves an unvalidated block into the blockchain to be used with Casper
func (cs *ConnectionStore) SaveBlock(block *protobufs.Block, shard uint32) error {
	res, _ := proto.Marshal(block)
	return cs.shardsChain[shard].BlockDb.Put([]byte(strconv.Itoa(int(block.GetIndex()))), res, nil)
}

// ImportBlock imports a block into the blockchain and checks if it's valid
// This should be called on blocks that are finalized by PoS
func (cs *ConnectionStore) ImportBlock(block *protobufs.Block, shard uint32) error {
	res, err := cs.shardsChain[shard].ValidateBlock(block)
	if !res {
		log.Error("ImportBlock ", err)
		return err
	}

	// The genesis block is a title of a The Times article, We still need to
	// add a validator because otherwise no blocks will be generated
	if block.GetIndex() == 0 {
		// TODO cange this import wallet because we don't want that people know the private key of those 2 wallet
		fakeTransaction := &protobufs.Transaction{}
		fakeTransaction.Amount = 10000
		state := &protobufs.AccountState{
			Balance: 20000,
		}

		state.Balance++
		wallet1, _ := wallet.ImportWallet("wallet1")
		cs.shardsChain[shard].SetState("Dexm013PiLMNGVKNadoqwtSw7YRtSrz4UTBD913CA2", state)
		cs.beaconChain.Validators.AddValidator("Dexm013PiLMNGVKNadoqwtSw7YRtSrz4UTBD913CA2", -300, wallet1.GetPublicKeySchnorrByte(), fakeTransaction, 1)

		state.Balance++
		wallet2, _ := wallet.ImportWallet("wallet2")
		cs.shardsChain[shard].SetState("Dexm013gEt9eb3bGYTgZ2qxDcyutqqN3dbD5A02B8D", state)
		cs.beaconChain.Validators.AddValidator("Dexm013gEt9eb3bGYTgZ2qxDcyutqqN3dbD5A02B8D", -300, wallet2.GetPublicKeySchnorrByte(), fakeTransaction, 2)

		state.Balance++
		wallet3, _ := wallet.ImportWallet("wallet3")
		cs.shardsChain[shard].SetState("Dexm014WmVd63Vn7aWYNRqNN4x7bATPTZ533213654", state)
		cs.beaconChain.Validators.AddValidator("Dexm014WmVd63Vn7aWYNRqNN4x7bATPTZ533213654", -300, wallet3.GetPublicKeySchnorrByte(), fakeTransaction, 3)

		state.Balance++
		wallet4, _ := wallet.ImportWallet("wallet4")
		cs.shardsChain[shard].SetState("Dexm014JwVstwpuarTFEPJmYjDFeRyphtS96055C71", state)
		cs.beaconChain.Validators.AddValidator("Dexm014JwVstwpuarTFEPJmYjDFeRyphtS96055C71", -300, wallet4.GetPublicKeySchnorrByte(), fakeTransaction, 4)

		state.Balance++
		wallet5, _ := wallet.ImportWallet("wallet5")
		cs.shardsChain[shard].SetState("Dexm012o1Rf35adWwZF3FKZyb2R77mpLJH185FEBB6", state)
		cs.beaconChain.Validators.AddValidator("Dexm012o1Rf35adWwZF3FKZyb2R77mpLJH185FEBB6", -300, wallet5.GetPublicKeySchnorrByte(), fakeTransaction, 5)

		cs.shardsChain[shard].GenesisTimestamp = block.GetTimestamp()

		return nil
	}

	// generate a seed to generate a shard for the new validators
	byteBlock, _ := proto.Marshal(block)
	hash := sha256.Sum256(byteBlock)
	seed := binary.BigEndian.Uint64(hash[:])
	rand.Seed(int64(seed))

	for _, t := range block.GetTransactions() {
		sender := wallet.BytesToAddress(t.GetSender(), t.GetShard())

		log.Info("Sender:", sender)
		log.Info("Recipient:", t.GetRecipient())
		log.Info("Amnt:", t.GetAmount())

		senderBalance, err := cs.shardsChain[shard].GetWalletState(sender)
		if err != nil {
			log.Error(err)
			continue
		}

		if t.GetRecipient() == "DexmPoS" {
			randomShard := uint32(rand.Int31n(nShard) + 1)
			exist := cs.beaconChain.Validators.AddValidator(sender, int64(cs.shardsChain[shard].CurrentBlock), t.GetPubSchnorrKey(), t, randomShard)
			if exist {
				log.Info("slash for ", sender)
				continue
			}
		}

		// Ignore error because if the wallet doesn't exist yet we don't care
		reciverBalance, _ := cs.shardsChain[shard].GetWalletState(t.GetRecipient())

		// No overflow checks because ValidateBlock already does that
		senderBalance.Balance -= t.GetAmount() + uint64(t.GetGas())
		reciverBalance.Balance += t.GetAmount()

		log.Info("Sender balance:", senderBalance.Balance)
		log.Info("Reciver balance:", reciverBalance.Balance)

		err = cs.shardsChain[shard].SetState(sender, &senderBalance)
		if err != nil {
			log.Error(err)
			continue
		}
		err = cs.shardsChain[shard].SetState(t.GetRecipient(), &reciverBalance)
		if err != nil {
			log.Error(err)
			continue
		}

		if t.GetContractCreation() {
			// Use sender a nonce to find contract address
			contractAddr := wallet.BytesToAddress([]byte(fmt.Sprintf("%s%d", sender, t.GetNonce())), t.GetShard())

			// Save it on a separate db
			log.Info("New contract at ", contractAddr)
			cs.shardsChain[shard].ContractDb.Put([]byte(contractAddr), t.GetData(), nil)
		}

		// If a function identifier is specified then fetch the contract and execute
		if t.GetFunction() != "" {
			c, err := blockchain.GetContract(t.GetRecipient(), cs.shardsChain[shard], t)
			if err != nil {
				return err
			}

			c.ExecuteContract(t.GetFunction(), t.GetArgs())
			if err != nil {
				return err
			}

			c.SaveState()
		}

		res, _ := proto.Marshal(t)
		dbKeyS := sha256.Sum256(res)
		dbKey := dbKeyS[:]

		// Remove the transaction from the DB and replace it with the block index
		cs.shardsChain[shard].BlockDb.Delete(dbKey, nil)
		cs.shardsChain[shard].BlockDb.Put(dbKey, []byte(string(block.GetIndex())), nil)
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
