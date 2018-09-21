package networking

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"reflect"
	"time"

	"github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/wallet"
	protoBlockchain "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/dexm-coin/protobufs/build/network"
	protoNetwork "github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

const (
	maxMessagesSave = 500
)

// ConnectionStore handles peer messaging
type ConnectionStore struct {
	clients   map[*client]bool
	broadcast chan []byte

	register   chan *client
	unregister chan *client

	beaconChain *blockchain.BeaconChain
	shardChain  *blockchain.Blockchain

	identity *wallet.Wallet
	network  string
}

type client struct {
	conn      *websocket.Conn
	send      chan []byte
	readOther chan []byte
	store     *ConnectionStore
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// StartServer creates a new ConnectionStore, which handles network peers
func StartServer(port, network string, shardChain *blockchain.Blockchain, beaconChain *blockchain.BeaconChain, idn *wallet.Wallet) (*ConnectionStore, error) {
	store := &ConnectionStore{
		clients:     make(map[*client]bool),
		broadcast:   make(chan []byte),
		register:    make(chan *client),
		unregister:  make(chan *client),
		beaconChain: beaconChain,
		shardChain:  shardChain,
		identity:    idn,
		network:     network,
	}

	// Hub that handles registration and unregistrations of clients
	go store.run()
	log.Info("Starting server on port ", port)

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		log.Info("New connection")
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		c := client{
			conn:      conn,
			send:      make(chan []byte, 256),
			readOther: make(chan []byte, 256),
			store:     store,
		}

		store.register <- &c

		go c.read()
		go c.write()
	})

	// Server that allows peers to connect
	go http.ListenAndServe(port, nil)

	return store, nil
}

// Connect connects to a server and adds it to the connectionStore
func (cs *ConnectionStore) Connect(ip string) error {
	dial := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dial.Dial(fmt.Sprintf("ws://%s/ws", ip), nil)
	if err != nil {
		return err
	}

	c := client{
		conn:      conn,
		send:      make(chan []byte, 256),
		readOther: make(chan []byte, 256),
		store:     cs,
	}

	cs.register <- &c

	go c.read()
	go c.write()

	return nil
}

// save the 100's hash of newest messages (without the ttl) arrived
// TODO If 2 blocks are found with the same index and same validator then slash
var hashMessages = make([][]byte, 0)

// run is the event handler to update the ConnectionStore
func (cs *ConnectionStore) run() {
	for {
		select {
		// A new client has registered
		case client := <-cs.register:
			cs.clients[client] = true

		// A client has quit, check if it exisited and delete it
		case client := <-cs.unregister:
			if _, ok := cs.clients[client]; ok {
				delete(cs.clients, client)
				close(client.send)
			}

		// Network wide broadcast. For now this uses a very simple and broken
		// algorithm but it could be optimized using ASNs as an overlay network
		case message := <-cs.broadcast:
			// log.Info("Message arrived")
			env := &network.Envelope{}
			broadcast := &network.Broadcast{}
			proto.Unmarshal(message, env)
			proto.Unmarshal(env.Data, broadcast)

			broadcast.TTL--
			if broadcast.TTL < 1 || broadcast.TTL > 64 {
				log.Info("skip message TTL")
				continue
			}

			broadcastBytes, err := proto.Marshal(broadcast)
			if err != nil {
				log.Error(err)
				continue
			}
			newEnv := &network.Envelope{
				Type: env.Type,
				Data: broadcastBytes,
			}
			data, err := proto.Marshal(newEnv)
			if err != nil {
				log.Error(err)
				continue
			}

			if len(cs.clients) < 1 {
				continue
			}
			y := math.Exp(float64(20/len(cs.clients))) - 0.5
			for k := range cs.clients {
				if rand.Float64() > y {
					continue
				}
				k.send <- data
			}
		}
	}
}

func checkDuplicatedMessage(msg []byte) bool {
	env := &network.Envelope{}
	broadcast := &network.Broadcast{}
	proto.Unmarshal(msg, env)
	proto.Unmarshal(env.Data, broadcast)

	// set TTL to 0, calculate the hash of the message, check if already exist
	copyBroadcast := *broadcast
	copyBroadcast.TTL = 0
	bhash := sha256.Sum256([]byte(fmt.Sprintf("%v", copyBroadcast)))
	hash := bhash[:]
	alreadyReceived := false
	for _, h := range hashMessages {
		equal := reflect.DeepEqual(h, hash)
		if equal {
			alreadyReceived = true
			break
		}
	}
	if alreadyReceived {
		// log.Info("skip message")
		return true
	}
	hashMessages = append(hashMessages, hash)
	if len(hashMessages) > maxMessagesSave {
		hashMessages = hashMessages[1:]
	}
	return false
}

// read reads data from the socket and handles it
func (c *client) read() {

	// Unregister if the node dies
	defer func() {
		log.Info("Client died")
		c.store.unregister <- c
		c.conn.Close()
	}()

	for {
		msgType, msg, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		if msgType != websocket.BinaryMessage {
			continue
		}

		pb := protoNetwork.Envelope{}
		err = proto.Unmarshal(msg, &pb)

		if err != nil {
			log.Error(err)
			continue
		}

		switch pb.GetType() {

		case protoNetwork.Envelope_BROADCAST:
			skip := checkDuplicatedMessage(msg)
			if skip {
				continue
			}

			go c.store.handleBroadcast(pb.GetData())
			c.store.broadcast <- msg

		// If the ContentType is a request then try to parse it as such and handle it
		case protoNetwork.Envelope_REQUEST:
			request := protoNetwork.Request{}
			err := proto.Unmarshal(pb.GetData(), &request)
			if err != nil {
				log.Error(err)
				continue
			}

			// Free up the goroutine to recive multi part messages
			go func() {
				env := protoNetwork.Envelope{}
				rawMsg := c.store.handleMessage(&request, c)

				env.Type = protoNetwork.Envelope_OTHER
				env.Data = rawMsg

				toSend, err := proto.Marshal(&env)
				if err != nil {
					log.Error(err)
					return
				}

				c.send <- toSend
			}()

		// Other data can be channeled so other parts of code can use it
		case protoNetwork.Envelope_OTHER:
			c.readOther <- pb.GetData()
		}
	}
}

// write checks the channel for data to write and writes it to the socket
func (c *client) write() {
	for {
		toWrite := <-c.send

		c.conn.WriteMessage(websocket.BinaryMessage, toWrite)
	}
}

// ValidatorLoop updates the current expected validator and generates a block
// if the validator has the same identity as the node generates a block
func (cs *ConnectionStore) ValidatorLoop() {
	if int64(cs.shardChain.GenesisTimestamp) > time.Now().Unix() {
		log.Info("Waiting for genesis")
		time.Sleep(time.Duration(int64(cs.shardChain.GenesisTimestamp)-time.Now().Unix()) * time.Second)
	}

	wal, err := cs.identity.GetWallet()
	if err != nil {
		log.Fatal(err)
	}

	signSequence := make(map[int64]string)
	counterSigning := int64(-1)
	totalCounterValidator := int64(-1)

	for {
		// The validator changes every time the unix timestamp is a multiple of 5
		// time.Sleep(time.Duration(4-time.Now().Unix()%10) * time.Second)
		time.Sleep(5 * time.Second)

		// check if the block with index cs.shardChain.CurrentBlock have been saved, otherwise save an empty block
		selectedBlock, err := cs.shardChain.GetBlock(cs.shardChain.CurrentBlock)
		if err != nil {
			bhash := sha256.Sum256(selectedBlock)
			hash := bhash[:]
			block := &protoBlockchain.Block{
				Index:     cs.shardChain.CurrentBlock,
				Timestamp: uint64(time.Now().Unix()),
				Miner:     "",
				PrevHash:  hash,
			}

			err = cs.shardChain.SaveBlock(block)
			if err != nil {
				log.Error(err)
			}
			err = cs.shardChain.ImportBlock(block)
			if err != nil {
				log.Error(err)
			}
		}

		cs.shardChain.CurrentBlock++

		// Change shard
		if cs.shardChain.CurrentBlock%10001 == 0 {
			// calulate the hash of the previous 100 blocks from current block - 1
			var hashBlocks []byte
			latestBlock := true

			for i := cs.shardChain.CurrentBlock - 1; i > cs.shardChain.CurrentBlock-100; i-- {
				currentBlockByte, err := cs.shardChain.GetBlock(i)
				if err != nil {
					log.Error(err)
				}
				currentBlock := &protoBlockchain.Block{}
				proto.Unmarshal(currentBlockByte, currentBlock)

				if latestBlock {
					bhash := sha256.Sum256(currentBlockByte)
					hashBlocks = append(hashBlocks, bhash[:]...)
					latestBlock = false
				}

				hashBlocks = append(hashBlocks, currentBlock.GetPrevHash()...)
			}
			finalHash := sha256.Sum256(hashBlocks)

			// choose the next shard with a seed
			seed := binary.BigEndian.Uint64(finalHash[:])
			newShard, err := cs.shardChain.Validators.ChooseShard(int64(seed), wal)
			if err != nil {
				log.Fatal(err)
			}

			// TODO like this doesn't work, you should remove the blockchain so early
			// remove the older blockchain and create a new one
			os.RemoveAll(".dexm")
			os.MkdirAll(".dexm", os.ModePerm)
			cs.shardChain, err = blockchain.NewBlockchain(".dexm/", 0)
			if err != nil {
				log.Fatal("NewBlockchain ", err)
			}
			// ask for the chain that corrispond to newShard shard
			cs.UpdateChain(newShard)
		}

		// every 30 block do the merkle root signature
		if cs.shardChain.CurrentBlock%31 == 0 {
			signSequence, totalCounterValidator = cs.beaconChain.Validators.ChooseSignSequence(int64(cs.shardChain.CurrentBlock))
			counterSigning = 0
		}
		// check if the it's your turn to make the signature
		if counterSigning < totalCounterValidator {
			if signSequence[counterSigning] == wal {
				// check if i'm the first to sign the merkle root
				if cs.beaconChain.CurrentSign == 0 || cs.beaconChain.SignedMerkleRoot == nil {
					schnorrPrivateKey, err := cs.beaconChain.Validators.GetSchnorrPrivateKey()
					if err != nil {
						log.Error(err)
					}

					var transactions []*protobufs.Transaction
					var merkleRootTransactionArray [][]byte
					var merkleRootReceiptArray [][]byte
					// cs.shardChain.CurrentBlock-counterSigning because maybe the real first one didn't sign and so on
					for i := cs.shardChain.CurrentBlock - counterSigning; i > cs.shardChain.CurrentBlock-counterSigning-30; i-- {
						blockByte, err := cs.shardChain.GetBlock(uint64(i))
						if err != nil {
							log.Error(err)
						}
						block := &protoBlockchain.Block{}
						proto.Unmarshal(blockByte, block)

						transactions = append(transactions, block.GetTransactions())

						merkleRootTransaction := block.GetMerkleRootTransaction()
						merkleRootReceipt := block.GetMerkleRootReceipt()

						signatureTransaction := wallet.Sign(merkleRootTransaction, schnorrPrivateKey)
						signatureReceipt := wallet.Sign(merkleRootReceipt, schnorrPrivateKey)

						merkleRootTransactionArray = append(merkleRootTransactionArray, []byte(fmt.Sprintf("%v", signatureTransaction)))
						merkleRootReceiptArray = append(merkleRootReceiptArray, []byte(fmt.Sprintf("%v", signatureReceipt)))
					}

					currentShard := cs.beaconChain.Validators.GetShard(wal)
					validatorsSign := []string{}
					for _, value := range signSequence {
						validatorsSign = append(validatorsSign, value)
					}

					mr := &protoBlockchain.MerkleRoot(currentShard, merkleRootTransactionArray, merkleRootReceiptArray, validatorsSign, transactions)
					mrByte, _ := proto.Marshal(mr)

					broadcastMr := &network.Broadcast{
						Type: Broadcast_MERKLE_ROOTS,
						TTL:  64,
						Data: mrByte,
					}
					broadcastMrByte, _ := proto.Marshal(broadcastMr)

					env := &network.Envelope{
						Type:  network.Envelope_BROADCAST,
						Data:  broadcastMrByte,
						Shard: currentShard,
					}

					data, _ := proto.Marshal(env)
					cs.broadcast <- data

				} else {
					merkleRoot := cs.beaconChain.SignedMerkleRoot
					
					var merkleRootTransactionArray [][]byte
					var merkleRootReceiptArray [][]byte
					mrTransaction := merkleRoot.GetSignedMerkleRootsTransaction()
					mrReceipt := merkleRoot.GetSignedMerkleRootsReceipt()
					for i := range len(mrTransaction) {
						signatureTransaction := wallet.Sign(mrTransaction[i], schnorrPrivateKey)
						signatureReceipt := wallet.Sign(mrReceipt[i], schnorrPrivateKey)
						
						merkleRootTransactionArray = append(merkleRootTransactionArray, []byte(fmt.Sprintf("%v", signatureTransaction)))
						merkleRootReceiptArray = append(merkleRootReceiptArray, []byte(fmt.Sprintf("%v", signatureReceipt)))
					}

					currentShard := cs.beaconChain.Validators.GetShard(wal)
					validatorsSign := []string{}
					for _, value := range signSequence {
						validatorsSign = append(validatorsSign, value)
					}

					mr := &protoBlockchain.MerkleRoot(currentShard, merkleRootTransactionArray, merkleRootReceiptArray, validatorsSign, transactions)
					mrByte, _ := proto.Marshal(mr)

					broadcastMr := &network.Broadcast{
						Type: Broadcast_MERKLE_ROOTS,
						TTL:  64,
						Data: mrByte,
					}
					broadcastMrByte, _ := proto.Marshal(broadcastMr)

					env := &network.Envelope{
						Type:  network.Envelope_BROADCAST,
						Data:  broadcastMrByte,
						Shard: currentShard,
					}

					data, _ := proto.Marshal(env)
					cs.broadcast <- data
				}
				cs.beaconChain.CurrentSign = counterSigning
			}
			counterSigning++
		}

		// TODO
		// reset at the end counterSigning , cs.beaconChain.CurrentSign , totalCounterValidator , CurrentMerkleRootSigned

		// Checkpoint Agreement
		if cs.shardChain.CurrentBlock%101 == 0 && cs.shardChain.CurrentBlock%10001 != 0 {
			// check if it is a validator, also check that the dynasty are correct
			if cs.shardChain.Validators.CheckDynasty(wal, cs.shardChain.CurrentBlock) {
				// get source and target block in the blockchain
				souceBlockByte, err := cs.shardChain.GetBlock(cs.shardChain.CurrentCheckpoint)
				if err != nil {
					log.Fatal("Get block ", err)
					continue
				}
				targetBlockByte, err := cs.shardChain.GetBlock(cs.shardChain.CurrentBlock - 1)
				if err != nil {
					log.Fatal("Get block ", err)
					continue
				}

				source := &protoBlockchain.Block{}
				target := &protoBlockchain.Block{}
				proto.Unmarshal(souceBlockByte, source)
				proto.Unmarshal(targetBlockByte, target)

				bhash1 := sha256.Sum256(souceBlockByte)
				hashSource := bhash1[:]
				bhash2 := sha256.Sum256(targetBlockByte)
				hashTarget := bhash2[:]

				vote := blockchain.CreateVote(hashSource, hashTarget, cs.shardChain.CurrentCheckpoint, cs.shardChain.CurrentBlock-1, cs.identity)

				// read all the incoming vote and store it, after 1 minut call CheckpointAgreement
				go func() {
					currentBlockCheckpoint := cs.shardChain.CurrentBlock - 1
					time.Sleep(1 * time.Minute)
					// time.Sleep(15 * time.Second)
					check := blockchain.CheckpointAgreement(cs.shardChain, cs.shardChain.CurrentCheckpoint, currentBlockCheckpoint)
					log.Info("CheckpointAgreement ", check)
				}()

				voteBytes, _ := proto.Marshal(&vote)
				pub, _ := cs.identity.GetPubKey()
				bhash := sha256.Sum256(voteBytes)
				hash := bhash[:]

				r, s, err := cs.identity.Sign(hash)
				if err != nil {
					log.Error(err)
					continue
				}
				signature := &network.Signature{
					Pubkey: pub,
					R:      r.Bytes(),
					S:      s.Bytes(),
					Data:   hash,
				}

				broadcast := &network.Broadcast{
					Data:     voteBytes,
					Type:     network.Broadcast_CHECKPOINT_VOTE,
					Identity: signature,
					TTL:      64,
				}

				broadcastBytes, _ := proto.Marshal(broadcast)

				env := &network.Envelope{
					Type: network.Envelope_BROADCAST,
					Data: broadcastBytes,
				}

				data, _ := proto.Marshal(env)

				cs.broadcast <- data
			}
		}

		validator, err := cs.shardChain.Validators.ChooseValidator(int64(cs.shardChain.CurrentBlock))
		if err != nil {
			log.Fatal(err)
			continue
		}

		// Start accepting the block from the new validator
		cs.shardChain.CurrentValidator = validator

		// If this node is the validator then generate a block and sign it
		if wal == validator {
			block, err := cs.shardChain.GenerateBlock(wal)
			if err != nil {
				log.Fatal(err)
				return
			}

			// Get marshaled block
			blockBytes, _ := proto.Marshal(block)

			// Sign the new block
			pub, _ := cs.identity.GetPubKey()
			bhash := sha256.Sum256(blockBytes)
			hash := bhash[:]
			r, s, err := cs.identity.Sign(hash)
			if err != nil {
				log.Error(err)
				return
			}

			signature := &network.Signature{
				Pubkey: pub,
				R:      r.Bytes(),
				S:      s.Bytes(),
				Data:   hash,
			}

			// Create a broadcast message and send it to the network
			broadcast := &network.Broadcast{
				Data:     blockBytes,
				Type:     network.Broadcast_BLOCK_PROPOSAL,
				Identity: signature,
				TTL:      64,
			}

			broadcastBytes, _ := proto.Marshal(broadcast)

			env := &network.Envelope{
				Data: broadcastBytes,
				Type: network.Envelope_BROADCAST,
			}

			data, _ := proto.Marshal(env)

			cs.broadcast <- data
		}
	}
}
