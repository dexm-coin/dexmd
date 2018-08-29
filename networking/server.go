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
	"github.com/dexm-coin/protobufs/build/network"
	protobufs "github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

const (
	maxMessagesSave = 100
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
			env := &network.Envelope{}
			broadcast := &network.Broadcast{}
			proto.Unmarshal(message, env)
			proto.Unmarshal(env.Data, broadcast)

			broadcast.TTL--
			if broadcast.TTL < 1 || broadcast.TTL > 32 {
				continue
			}

			// set TTL to 0, calculate the hash of the message, check if already exist
			copyBroadcast := broadcast
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
				continue
			}
			hashMessages = append(hashMessages, hash)
			if len(hashMessages) > maxMessagesSave {
				hashMessages = hashMessages[1:]
			}

			broadcastBytes, err := proto.Marshal(broadcast)
			if err != nil {
				log.Error(err)
				return
			}
			newEnv := &network.Envelope{
				Type: env.Type,
				Data: broadcastBytes,
			}
			data, err := proto.Marshal(newEnv)
			if err != nil {
				log.Error(err)
				return
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

		pb := protobufs.Envelope{}
		err = proto.Unmarshal(msg, &pb)

		if err != nil {
			log.Error(err)
			continue
		}
		log.Printf("Recived message(%d): %x", pb.GetType(), msg)

		switch pb.GetType() {

		case protobufs.Envelope_BROADCAST:
			go c.store.handleBroadcast(pb.GetData())
			c.store.broadcast <- msg

		// If the ContentType is a request then try to parse it as such and handle it
		case protobufs.Envelope_REQUEST:
			request := protobufs.Request{}
			err := proto.Unmarshal(pb.GetData(), &request)
			if err != nil {
				continue
			}

			// Free up the goroutine to recive multi part messages
			go func() {
				env := protobufs.Envelope{}
				rawMsg := c.store.handleMessage(&request, c)

				env.Type = protobufs.Envelope_OTHER
				env.Data = rawMsg

				toSend, err := proto.Marshal(&env)
				if err != nil {
					return
				}

				c.send <- toSend
			}()

		// Other data can be channeled so other parts of code can use it
		case protobufs.Envelope_OTHER:
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

		validator, err := cs.bc.Validators.ChooseValidator(int64(cs.bc.CurrentBlock))
		if err != nil {
			log.Fatal(err)
		}

		// Start accepting the block from the new validator
		cs.bc.CurrentValidator = validator

		wal, err := cs.identity.GetWallet()
		if err != nil {
			log.Fatal(err)
		}

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
				TTL:      32,
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
