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

	// "bytes"
	// "encoding/gob"

	"gopkg.in/dedis/kyber.v2"
	// "gopkg.in/dedis/kyber.v2/group/edwards25519"

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
				Type:  env.Type,
				Data:  broadcastBytes,
				Shard: env.Shard,
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
			if pb.GetShard() != -1 {
				wallet, err := c.store.identity.GetWallet()
				if err != nil {
					log.Error(err)
					continue
				}
				currentShard, err := c.store.beaconChain.Validators.GetShard(wallet)
				if err != nil {
					log.Error(err)
					continue
				}
				if pb.GetShard() != currentShard {
					log.Info("Not your shard")
					continue
				}
			}

			request := protoNetwork.Request{}
			err = proto.Unmarshal(pb.GetData(), &request)
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
				env.Shard = -1

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

	var currentK kyber.Scalar

	for {
		// The validator changes every time the unix timestamp is a multiple of 5
		// time.Sleep(time.Duration(4-time.Now().Unix()%10) * time.Second)
		time.Sleep(5 * time.Second)

		currentShard, err := cs.beaconChain.Validators.GetShard(wal)
		if err != nil {
			log.Error(err)
			continue
		}

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
			err = cs.ImportBlock(block)
			if err != nil {
				log.Error(err)
			}
		}

		cs.shardChain.CurrentBlock++
		log.Info("Current block ", cs.shardChain.CurrentBlock)

		// Change shard
		if cs.shardChain.CurrentBlock%10001 == 0 {
			// calulate the hash of the previous 100 blocks from current block - 1
			var hashBlocks []byte
			latestBlock := true

			for i := cs.shardChain.CurrentBlock - 1; i > cs.shardChain.CurrentBlock-10; i-- {
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
			newShard, err := cs.beaconChain.Validators.ChooseShard(int64(seed), wal)
			if err != nil {
				log.Fatal(err)
			}

			// TODO like this doesn't work, you shouldn't remove the blockchain so early

			// remove the older blockchain and create a new one
			os.RemoveAll(".dexm/shard")
			os.MkdirAll(".dexm/shard", os.ModePerm)
			cs.shardChain, err = blockchain.NewBlockchain(".dexm/shard/", 0)
			if err != nil {
				log.Fatal("NewBlockchain ", err)
			}
			// ask for the chain that corrispond to newShard shard
			cs.UpdateChain(newShard)
		}

		// every 30 blocks do the merkle root signature
		if cs.shardChain.CurrentBlock%2 == 0 {
			cs.beaconChain.CurrentSign = cs.beaconChain.Validators.ChooseSignSequence(int64(cs.shardChain.CurrentBlock))

			// generate k and caluate r
			k, rByte, err := wallet.GenerateParameter()
			if err != nil {
				log.Error(err)
			}
			currentK = k

			schnorrP := &protoBlockchain.Schnorr{
				R: rByte,
				P: wal,
			}
			schnorrPByte, _ := proto.Marshal(schnorrP)

			// broadcast schnorr message
			broadcastSchnorr := &network.Broadcast{
				Type: protoNetwork.Broadcast_SCHNORR,
				TTL:  64,
				Data: schnorrPByte,
			}
			broadcastSchnorrByte, _ := proto.Marshal(broadcastSchnorr)

			env := &network.Envelope{
				Type:  network.Envelope_BROADCAST,
				Data:  broadcastSchnorrByte,
				Shard: currentShard,
			}

			data, _ := proto.Marshal(env)
			cs.broadcast <- data
		}

		// after 3 turn, make the sign and broadcast it
		if cs.shardChain.CurrentBlock%4 == 0 {
			x, err := cs.identity.GetPrivateKeySchnorr()
			if err != nil {
				log.Error(err)
			}

			var myR []byte
			var myP []byte
			var Rs []kyber.Point
			var Ps []kyber.Point

			log.Info("CurrentSign ", cs.beaconChain.CurrentSign)
			log.Info("cs.shardChain.Schnorr ", cs.shardChain.Schnorr)
			// 25 is the total number of validator that should sign
			for i := int64(0); i < 25; i++ {
				if _, ok := cs.beaconChain.CurrentSign[i]; ok {
					if _, ok := cs.shardChain.Schnorr[cs.beaconChain.CurrentSign[i]]; ok {
						log.Info("Index ", i)

						r, err := wallet.ByteToPoint(cs.shardChain.Schnorr[cs.beaconChain.CurrentSign[i]])
						if err != nil {
							log.Error("r ByteToPoint ", err)
						}
						log.Info("ok1")
						if cs.beaconChain.CurrentSign[i] == wal {
							myR, err = r.MarshalBinary()
							if err != nil {
								log.Error("r marshal ", err)
							}
							log.Info("ok2")
							p, err := cs.beaconChain.Validators.GetSchnorrPublicKey(wal)
							if err != nil {
								log.Error("GetSchnorrPublicKey ", err)
							}
							log.Info("ok3")
							myP, err = p.MarshalBinary()
							if err != nil {
								log.Error("p marshal ", err)
							}
							log.Info("ok4")
							continue
						}
						log.Info("ok5")
						Rs = append(Rs, r)
						p, err := cs.beaconChain.Validators.GetSchnorrPublicKey(cs.beaconChain.CurrentSign[i])
						if err != nil {
							log.Error("GetSchnorrPublicKey ", err)
						}
						log.Info("ok6")
						Ps = append(Ps, p)
					}
				}
			}
			log.Info("Rs ", Rs)
			log.Info("Ps ", Ps)

			stringMRTransaction := ""
			stringMRReceipt := ""
			// -3 becuase it's after 3 turn from sending GenerateParameter
			for i := int64(cs.shardChain.CurrentBlock) - 3; i > int64(cs.shardChain.CurrentBlock)-33; i-- {
				blockByte, err := cs.shardChain.GetBlock(uint64(i))
				if err != nil {
					log.Error(err)
					continue
				}
				block := &protoBlockchain.Block{}
				proto.Unmarshal(blockByte, block)

				merkleRootTransaction := block.GetMerkleRootTransaction()
				merkleRootReceipt := block.GetMerkleRootReceipt()
				stringMRTransaction += string(merkleRootTransaction)
				stringMRReceipt += string(merkleRootReceipt)
			}

			signTransaction := wallet.MakeSign(x, currentK, stringMRTransaction, Rs, Ps)
			signReceipt := wallet.MakeSign(x, currentK, stringMRReceipt, Rs, Ps)

			// send the signed transaction and receipt
			signSchorrP := &protoBlockchain.SignSchnorr{
				Shard:                  currentShard,
				Wallet:                 wal,
				RSchnorr:               myR,
				PSchnorr:               myP,
				SignTransaction:        signTransaction,
				SignReceipt:            signReceipt,
				MessageSignTransaction: stringMRTransaction,
				MessageSignReceipt:     stringMRReceipt,
			}
			signSchorrByte, _ := proto.Marshal(signSchorrP)

			broadcastSignSchorr := &network.Broadcast{
				Type: protoNetwork.Broadcast_SIGN_SCHNORR,
				TTL:  64,
				Data: signSchorrByte,
			}
			broadcastMrByte, _ := proto.Marshal(broadcastSignSchorr)

			env := &network.Envelope{
				Type:  network.Envelope_BROADCAST,
				Data:  broadcastMrByte,
				Shard: currentShard,
			}

			data, _ := proto.Marshal(env)
			cs.broadcast <- data
		}

		// after another 3 turn make the final signature, from sign of the chosen validator, and verify it
		if cs.shardChain.CurrentBlock%6 == 0 {
			// cs.shardChain.MTTrasaction
			// cs.shardChain.MTReceipt
			// cs.shardChain.RSchnorr
			// cs.shardChain.PSchnorr
			// cs.shardChain.MessagesTransaction
			// cs.shardChain.MessagesReceipt

			var Rs []kyber.Point
			var Ps []kyber.Point
			var SsTransaction []kyber.Scalar
			var SsReceipt []kyber.Scalar
			var MessagesTransaction []string
			var MessagesReceipt []string

			// TODO change 2 with len(cs.shardChain.RSchnorr)
			for i := 0; i < 2; i++ {
				r, err := wallet.ByteToPoint(cs.shardChain.RSchnorr[i])
				if err != nil {
					log.Error(err)
				}
				p, err := wallet.ByteToPoint(cs.shardChain.PSchnorr[i])
				if err != nil {
					log.Error(err)
				}

				sTransaction, err := wallet.ByteToScalar(cs.shardChain.MTTrasaction[i])
				if err != nil {
					log.Error(err)
				}
				sReceipt, err := wallet.ByteToScalar(cs.shardChain.MTReceipt[i])
				if err != nil {
					log.Error(err)
				}

				Rs = append(Rs, r)
				Ps = append(Ps, p)
				SsTransaction = append(SsTransaction, sTransaction)
				SsReceipt = append(SsReceipt, sReceipt)
				MessagesTransaction = append(MessagesTransaction, cs.shardChain.MessagesTransaction[i])
				MessagesReceipt = append(MessagesReceipt, cs.shardChain.MessagesReceipt[i])
			}

			RsignatureTransaction, SsignatureTransaction, err := wallet.CreateSignature(Rs, SsTransaction)
			if err != nil {
				log.Error(err)
			}
			// RsignatureReceipt, SsignatureReceipt, err := wallet.CreateSignature(Rs, SsReceipt)
			// if err != nil {
			// 	log.Error(err)
			// }

			rSignedTransaction, err := wallet.ByteToPoint(RsignatureTransaction)
			if err != nil {
				log.Error(err)
			}
			sSignedTransaction, err := wallet.ByteToScalar(SsignatureTransaction)
			if err != nil {
				log.Error(err)
			}

			for _, message := range MessagesTransaction {
				verify := wallet.VerifySignature(message, rSignedTransaction, sSignedTransaction, Ps, Rs)
				// log.Info("Verify ", verify)
				if !verify {
					log.Error("Not verify")
				}
			}

			// Rs = append(Rs, myR)
			// var RsByte [][]byte
			// for i := 0; i < len(Rs); i++ {
			// 	res, err := Rs[i].MarshalBinary()
			// 	if err != nil {
			// 		log.Error(err)
			// 	}
			// 	RsByte = append(RsByte, res)
			// }

			// mr := &protoBlockchain.MerkleRoot{
			// 	Shard: currentShard,
			// 	MerkleRootsTransaction:        merkleRootTransactionArray,
			// 	MerkleRootsReceipt:            merkleRootReceiptArray,
			// 	RSignedMerkleRootsTransaction: RsignatureTransaction,
			// 	SSignedMerkleRootsTransaction: SsignatureTransaction,
			// 	RSignedMerkleRootsReceipt:     RsignatureReceipt,
			// 	SSignedMerkleRootsReceipt:     SsignatureReceipt,
			// 	RValidators:                   RsByte,
			// 	Validators:                    PartecipantValidator,
			// }
			// mrByte, _ := proto.Marshal(mr)

			// broadcastMr := &network.Broadcast{
			// 	Type: protoNetwork.Broadcast_MERKLE_ROOTS_SIGNED,
			// 	TTL:  64,
			// 	Data: mrByte,
			// }
			// broadcastMrByte, _ := proto.Marshal(broadcastMr)

			// env := &network.Envelope{
			// 	Type:  network.Envelope_BROADCAST,
			// 	Data:  broadcastMrByte,
			// 	Shard: currentShard,
			// }

			// data, _ := proto.Marshal(env)
			// cs.broadcast <- data

			for k := range cs.beaconChain.CurrentSign {
				delete(cs.beaconChain.CurrentSign, k)
			}
		}

		// Checkpoint Agreement
		if cs.shardChain.CurrentBlock%101 == 0 && cs.shardChain.CurrentBlock%10001 != 0 {
			// check if it is a validator, also check that the dynasty are correct
			if cs.beaconChain.Validators.CheckDynasty(wal, cs.shardChain.CurrentBlock) {
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

				vote := CreateVote(hashSource, hashTarget, cs.shardChain.CurrentCheckpoint, cs.shardChain.CurrentBlock-1, cs.identity)

				// read all the incoming vote and store it, after 1 minut call CheckpointAgreement
				go func() {
					currentBlockCheckpoint := cs.shardChain.CurrentBlock - 1
					time.Sleep(1 * time.Minute)
					// time.Sleep(15 * time.Second)
					check := cs.CheckpointAgreement(cs.shardChain.CurrentCheckpoint, currentBlockCheckpoint)
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
					Type:  network.Envelope_BROADCAST,
					Data:  broadcastBytes,
					Shard: currentShard,
				}

				data, _ := proto.Marshal(env)

				cs.broadcast <- data
			}
		}

		validator, err := cs.beaconChain.Validators.ChooseValidator(int64(cs.shardChain.CurrentBlock))
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
				Data:  broadcastBytes,
				Type:  network.Envelope_BROADCAST,
				Shard: currentShard,
			}

			data, _ := proto.Marshal(env)

			cs.broadcast <- data
		}
	}
}
