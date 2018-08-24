package networking

import (
	"net/http"
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
	maxRelays = 20
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

	conn, _, err := dial.Dial(ip, nil)
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
			sentBroadcasts := 0

			env := &network.Envelope{}
			broadcast := &network.Broadcast{}
			proto.Unmarshal(message, env)
			proto.Unmarshal(env.Data, broadcast)

			broadcast.TTL--
			if broadcast.TTL < 1 || broadcast.TTL > 32 {
				return
			}

			broadcastBytes, err := proto.Marshal(broadcast)
			if err != nil {
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

			for k := range cs.clients {
				// Limit messages that one node sends
				if sentBroadcasts > maxRelays {
					break
				}

				k.send <- data
				sentBroadcasts++
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
			err := proto.Unmarshal(msg, &request)
			if err != nil {
				continue
			}

			// Free up the goroutine to recive multi part messages
			go func() {
				env := protobufs.Envelope{}
				rawMsg := c.store.handleMessage(&request)

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
