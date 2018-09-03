package networking

import (
	"crypto/sha256"
	"fmt"
	"math"
	"math/rand"
	"net/http"
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
	clients map[*client]bool

	broadcast chan []byte

	register chan *client

	unregister chan *client

	bc *blockchain.Blockchain

	identity *wallet.Wallet

	network string
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
func StartServer(port, network string, bch *blockchain.Blockchain, idn *wallet.Wallet) (*ConnectionStore, error) {
	store := &ConnectionStore{
		clients:    make(map[*client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *client),
		unregister: make(chan *client),
		bc:         bch,
		identity:   idn,
		network:    network,
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
			log.Info("Message arrived")
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
		log.Info("skip message")
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
	if int64(cs.bc.GenesisTimestamp) > time.Now().Unix() {
		log.Info("Waiting for genesis")
		time.Sleep(time.Duration(int64(cs.bc.GenesisTimestamp)-time.Now().Unix()) * time.Second)
	}

	for {
		// The validator changes every time the unix timestamp is a multiple of 5
		time.Sleep(5 * time.Second)

		cs.bc.CurrentBlock++

		wal, err := cs.identity.GetWallet()
		if err != nil {
			log.Fatal(err)
		}

		if cs.bc.CurrentBlock%6 == 0 {
			// get source and target block in the blockchain
			souceBlockByte, err := cs.bc.GetBlocks(cs.bc.CurrentCheckpoint)
			if err != nil {
				log.Fatal(err)
			}
			targetBlockByte, err := cs.bc.GetBlocks(cs.bc.CurrentBlock - 1)
			if err != nil {
				log.Fatal(err)
			}

			souceBlock := &protoBlockchain.Index{}
			targetBlock := &protoBlockchain.Index{}
			proto.Unmarshal(souceBlockByte, souceBlock)
			proto.Unmarshal(targetBlockByte, targetBlock)

			if len(souceBlock.Blocks) < 1 || len(targetBlock.Blocks) < 1 {
				log.Fatal("blocks too short")
				continue
			}
			source := souceBlock.Blocks[0]
			target := targetBlock.Blocks[0]

			MarshalSource, err := proto.Marshal(source)
			if err != nil {
				log.Error(err)
				continue
			}
			bhash1 := sha256.Sum256(MarshalSource)
			hashSource := bhash1[:]
			MarshalTarget, err := proto.Marshal(target)
			if err != nil {
				log.Error(err)
				continue
			}
			bhash2 := sha256.Sum256(MarshalTarget)
			hashTarget := bhash2[:]

			vote := blockchain.CreateVote(hashSource, hashTarget, cs.bc.CurrentCheckpoint, cs.bc.CurrentBlock-1, cs.identity)

			// read all the incoming vote and store it, after 1 minut call CheckpointAgreement
			go func() {
				// time.Sleep(1*time.Minute)
				time.Sleep(15 * time.Second)
				blockchain.CheckpointAgreement(cs.bc, source, target)
			}()

			voteBytes, _ := proto.Marshal(&vote)
			pub, _ := cs.identity.GetPubKey()
			bhash := sha256.Sum256(voteBytes)
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

		validator, err := cs.bc.Validators.ChooseValidator(int64(cs.bc.CurrentBlock))
		if err != nil {
			log.Fatal(err)
		}

		// Start accepting the block from the new validator
		cs.bc.CurrentValidator = validator

		// If this node is the validator then generate a block and sign it
		if wal == validator {
			block, err := cs.bc.GenerateBlock(wal)
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
