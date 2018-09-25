package networking

import (
	"errors"
	"strconv"
	"time"

	"github.com/dexm-coin/dexmd/contracts"
	"github.com/dexm-coin/dexmd/wallet"
	"github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"

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
func (cs *ConnectionStore) UpdateChain(nextShard int64) error {
	// No need to sync before genesis
	for cs.shardChain.GetNetworkIndex() < 0 {
		return nil
	}

	for cs.shardChain.CurrentBlock <= uint64(cs.shardChain.GetNetworkIndex()) {
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

			cb := cs.shardChain.CurrentBlock
			if flen < cb {
				continue
			}

			// Download new blocks up to MaxBlockPeer
			for i := cb; i < min(cb+MaxBlockPeer, flen); i++ {
				req = &network.Request{
					Type:  network.Request_GET_BLOCK,
					Index: i,
				}

				d, err = makeReqEnvelope(req, currentShard)
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
					cs.ImportBlock(b)
					cs.shardChain.CurrentBlock++
				} else {
					log.Error("import ", err)
				}
			}
		}
	}

	return nil
}

// ImportBlock imports a block into the blockchain and checks if it's valid
// This should be called on blocks that are finalized by PoS
func (cs *ConnectionStore) ImportBlock(block *protobufs.Block) error {
	res, err := cs.shardChain.ValidateBlock(block)
	if !res {
		log.Error("ImportBlock ", err)
		return err
	}

	// The genesis block is a title of a The Times article, We still need to
	// add a validator because otherwise no blocks will be generated
	if block.GetIndex() == 0 {
		// A bit of balance to run tests
		state := &protobufs.AccountState{
			Balance: 20000,
			Nonce:   0,
		}

		cs.shardChain.SetState("Dexm3ENiLVMNwaeRswEbV1PT7UEpDNwwlbef2e683", state)
		cs.beaconChain.Validators.AddValidator("Dexm3ENiLVMNwaeRswEbV1PT7UEpDNwwlbef2e683", 20000, -300, []byte{})

		state = &protobufs.AccountState{
			Balance: 10000,
			Nonce:   0,
		}

		cs.shardChain.SetState("Dexm25g6YbMNWpu9LHqCTP7S8r2PHBMHla441f087", state)
		//TODO change []byte{}
		cs.beaconChain.Validators.AddValidator("Dexm25g6YbMNWpu9LHqCTP7S8r2PHBMHla441f087", 10000, -300, []byte{})

		cs.shardChain.GenesisTimestamp = block.GetTimestamp()

		return nil
	}

	totalGas := uint32(0)

	for _, t := range block.GetTransactions() {
		sender := wallet.BytesToAddress(t.GetSender())

		log.Info("Sender:", sender)
		log.Info("Recipient:", t.GetRecipient())
		log.Info("Amnt:", t.GetAmount())

		senderBalance, err := cs.shardChain.GetWalletState(sender)
		if err != nil {
			log.Error(err)
			return err
		}

		if t.GetRecipient() == "DexmPoS" {
			exist := cs.beaconChain.Validators.AddValidator(sender, t.GetAmount(), int64(cs.shardChain.CurrentBlock), t.GetPubSchnorrKey())
			if exist {
				log.Info("slash for ", sender)
			}
		}

		// Ignore error because if the wallet doesn't exist yet we don't care
		reciverBalance, _ := cs.shardChain.GetWalletState(t.GetRecipient())

		// No overflow checks because ValidateBlock already does that
		senderBalance.Balance -= t.GetAmount() + uint64(t.GetGas())
		reciverBalance.Balance += t.GetAmount()

		// Avoid replaying transactions
		senderBalance.Nonce++

		log.Info("Sender balance:", senderBalance.Balance)
		log.Info("Sender nonce: ", senderBalance.Nonce)
		log.Info("Reciver balance:", reciverBalance.Balance)

		totalGas += t.GetGas()

		err = cs.shardChain.SetState(sender, &senderBalance)
		if err != nil {
			log.Error(err)
			return err
		}
		err = cs.shardChain.SetState(t.GetRecipient(), &reciverBalance)
		if err != nil {
			log.Error(err)
			return err
		}

		if t.GetContractCreation() {
			// Use the code, sender and prev hash to decide contract address
			contractAddrSource := append(t.GetData(), t.GetSender()...)
			contractAddrSource = append(contractAddrSource, block.PrevHash...)
			contractAddr := wallet.BytesToAddress(contractAddrSource)

			// Save it on a separate db
			log.Info("New contract at ", contractAddr)
			cs.shardChain.ContractDb.Put([]byte(contractAddr), t.GetData(), nil)
		}

		// If a function identifier is specified then fetch the contract and execute
		if t.GetFunction() != "" {
			c, err := contracts.GetContract(t.GetRecipient(), cs.shardChain.ContractDb, cs.shardChain.StateDb)
			if err != nil {
				return err
			}

			err = c.ExecuteContract(t.GetFunction(), t.GetArgs())
			if err != nil {
				return err
			}

			c.SaveState()
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

func makeReqEnvelope(req *network.Request, currentShard int64) ([]byte, error) {
	d, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}

	env := &network.Envelope{
		Type:  network.Envelope_REQUEST,
		Data:  d,
		Shard: currentShard,
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
