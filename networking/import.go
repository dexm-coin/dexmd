package networking

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/util"
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
func (cs *ConnectionStore) UpdateChain(nextShard uint32, oldShard string) error {
	// No need to sync before genesis
	for cs.shardsChain[nextShard].GetNetworkIndex() <= 0 {
		return nil
	}

	w := cs.identity
	shardWallet := uint32(w.GetShardWallet())

	clientToDrop := []*client{}

	for cs.shardsChain[nextShard].CurrentBlock <= uint64(cs.shardsChain[nextShard].GetNetworkIndex()) {
		for k := range cs.clients {

			err := MakeSpecificRequest(w, nextShard, []byte{}, network.Request_GET_BLOCKCHAIN_LEN, k, shardWallet)
			if err != nil {
				log.Error(err)
				continue
			}

			blockchainLen, err := k.GetResponse(100 * time.Millisecond)
			if err != nil {
				clientToDrop = append(clientToDrop, k)
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

				err := MakeSpecificRequest(w, nextShard, byteI, network.Request_GET_BLOCK, k, shardWallet)
				if err != nil {
					log.Error(err)
					break
				}

				block, err := k.GetResponse(300 * time.Millisecond)
				if err != nil {
					clientToDrop = append(clientToDrop, k)
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
	}

	// remove the clients that didn't respond
	for _, k := range clientToDrop {
		if _, ok := cs.clients[k]; ok {
			delete(cs.clients, k)
		}
	}

	if oldShard == "" {
		return nil
	}

	jsonFile, err := os.OpenFile("config.json", os.O_RDONLY|os.O_CREATE, 0666)
	defer jsonFile.Close()
	if err != nil {
		log.Error(err)
		return err
	}
	data, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		log.Error(err)
		return err
	}
	var shardInterest []string
	err = json.Unmarshal(data, &shardInterest)
	if err != nil {
		log.Error(err)
		return err
	}

	// remove the blockchain in oldShard if isn't in the config.json
	remove := true
	for _, shardI := range shardInterest {
		if shardI == oldShard {
			remove = false
			break
		}
	}
	if remove {
		cs.RemoveInterest(oldShard)
		err := os.RemoveAll(".dexm/shard" + oldShard + "/")
		if err != nil {
			log.Error(err)
			return err
		}
	}

	return nil
}

func MakeSpecificRequest(w *wallet.Wallet, shard uint32, dataRequest []byte, t network.Request_MessageTypes, c *client, shardAddress uint32) error {
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
		return err
	}

	signature := &network.Signature{
		Pubkey: pubKey,
		R:      r.Bytes(),
		S:      s.Bytes(),
		Data:   hashRequest,
	}

	env := &network.Envelope{
		Data:     requestBytes,
		Type:     network.Envelope_REQUEST,
		Shard:    shard,
		Identity: signature,
		TTL:      64,
	}
	data, _ := proto.Marshal(env)

	err = c.conn.WriteMessage(websocket.BinaryMessage, data)
	if err != nil {
		log.Error(err)
		return err
	}

	c.send <- data
	return nil
}

func (cs *ConnectionStore) MakeEnvelopeBroadcast(dataBroadcast []byte, typeBroadcast network.Broadcast_BroadcastType, shardAddress uint32, shardEnvelope uint32) {
	pubKey, _ := cs.identity.GetPubKey()

	// Create a broadcast message and send it to the network
	broadcast := &network.Broadcast{
		Data:         dataBroadcast,
		Type:         typeBroadcast,
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
	cs.broadcast <- data
}

// func (cs *ConnectionStore) RequestHashBlock(shard uint32, indexBlock uint64, hashBlock []byte) (*protobufs.Block, bool) {

// 	// TODOMaybe non chiedo a tutti i client ma solo ai validator nella mia shard
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
// 			// TODOMaybe in base allo stake conta quanti ti hanno dato il blocco e quello giusto
// 		}
// 	}
// 	return nil, false
// }

// func (cs *ConnectionStore) RequestMissingBlock(shard uint32, indexBlock uint64) (*protobufs.Block, bool) {

// 	// TODOMaybe non chiedo a tutti i client ma solo ai validator nella mia shard
// 	for c := range cs.clients {
// 		byteI := make([]byte, 4)
// 		binary.LittleEndian.PutUint64(byteI, indexBlock)

// 		err := MakeSpecificRequest(shard, byteI, network.Request_GET_BLOCK, c.conn)
// 		if err != nil {
// 			log.Error(err)
// 			continue
// 		}

// 		// TODOMaybe the request of the block can be substitue with zk snark that give you the proof that this block exist
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
// 			// TODOMaybe in base allo stake conta quanti ti hanno dato il blocco e quello giusto
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
		// TODO remember to remove the private key from the wallet
		fakeTransaction := &protobufs.Transaction{}
		fakeTransaction.Amount = 10000
		state := &protobufs.AccountState{
			Balance: 20000,
		}

		state.Balance++
		wallet1, _ := wallet.ImportWallet("wallet1")
		address1, _ := wallet1.GetWallet()
		cs.shardsChain[shard].SetState(address1, state)
		cs.beaconChain.Validators.AddValidator(address1, -300, wallet1.GetPublicKeySchnorrByte(), fakeTransaction, 1)

		state.Balance++
		wallet2, _ := wallet.ImportWallet("wallet2")
		address2, _ := wallet2.GetWallet()
		cs.shardsChain[shard].SetState(address2, state)
		cs.beaconChain.Validators.AddValidator(address2, -300, wallet2.GetPublicKeySchnorrByte(), fakeTransaction, 2)

		state.Balance++
		wallet3, _ := wallet.ImportWallet("wallet3")
		address3, _ := wallet3.GetWallet()
		cs.shardsChain[shard].SetState(address3, state)
		cs.beaconChain.Validators.AddValidator(address3, -300, wallet3.GetPublicKeySchnorrByte(), fakeTransaction, 3)

		state.Balance++
		wallet4, _ := wallet.ImportWallet("wallet4")
		address4, _ := wallet4.GetWallet()
		cs.shardsChain[shard].SetState(address4, state)
		cs.beaconChain.Validators.AddValidator(address4, -300, wallet4.GetPublicKeySchnorrByte(), fakeTransaction, 4)

		state.Balance++
		wallet5, _ := wallet.ImportWallet("wallet5")
		address5, _ := wallet5.GetWallet()
		cs.shardsChain[shard].SetState(address5, state)
		cs.beaconChain.Validators.AddValidator(address5, -300, wallet5.GetPublicKeySchnorrByte(), fakeTransaction, 5)

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
				// The sender has already lost his money sent for the second time to DexmPoS
				continue
			}
		}

		// Ignore error because if the wallet doesn't exist yet we don't care
		reciverBalance, _ := cs.shardsChain[shard].GetWalletState(t.GetRecipient())

		// If a function identifier is specified then fetch the contract and execute
		// If the contract reverts take the gas and return the transaction value
		if t.GetFunction() != "" {
			c, err := blockchain.GetContract(t.GetRecipient(), cs.shardsChain[shard], t, senderBalance.Balance)
			if err != nil {
				return err
			}

			returnState := c.ExecuteContract(t.GetFunction(), t.GetArgs())
			if err != nil {
				return err
			}

			// Save the new state only if the contract hasn't reverted
			if !returnState.Reverted {
				c.SaveState()

				// Save the transactions which the contract outputted
				for _, v := range returnState.Outputs {
					recBal, _ := cs.shardsChain[shard].GetWalletState(v.Recipient)

					// Check if balance is sufficient
					requiredBal, ok := util.AddU64O(recBal.Balance, v.Amount)
					if requiredBal > recBal.Balance && ok {
						continue
					}
					recBal.Balance += v.Amount
					err = cs.shardsChain[shard].SetState(v.Recipient, &recBal)
					if err != nil {
						log.Error(err)
						continue
					}
				}
			} else {
				continue
			}
		}

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
