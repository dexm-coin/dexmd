package networking

import (
	"net/http"
	"time"

	"github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

type client struct {
	conn      *websocket.Conn
	identity  []byte
	send      chan []byte
	readOther chan []byte
	store     *ConnectionStore
}

// ConnectionStore handles peer messaging
type ConnectionStore struct {
	clients map[*client]bool

	broadcast chan []byte

	register chan *client

	unregister chan *client
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// StartServer creates a new ConnectionStore, which handles network peers
func StartServer(port string, bch *blockchain.Blockchain, idn *wallet.Wallet) (*ConnectionStore, error) {
	store := &ConnectionStore{
		clients:    make(map[*client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *client),
		unregister: make(chan *client),
	}

	bc = bch
	identity = idn

	// Hub that handles registration and unregistrations of clients
	go store.run()
	log.Info("Starting server on port ", port)

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		log.Info("New connection")
		registerWs(store, w, r)
	})

	// Server that allows peers to connect
	go http.ListenAndServe(port, nil)

	// Loop that picks the validator for the next block and waits for a block
	// signed by him
	go func() {
		for {
			wal, _ := identity.GetWallet()
			block, err := bc.GenerateBlock(wal)
			if err != nil {
				log.Error(err)
			}

			log.Printf("New block: %x", block)

			time.Sleep(5 * time.Second)
		}
	}()

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
		identity:  []byte{},
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
			for k := range cs.clients {
				k.send <- message
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
		msgType, msg, err := c.conn.ReadMessage() // TODO Handle go away messages
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
			go handleBroadcast(pb.GetData())
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
				rawMsg := handleMessage(&request)

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

		c.conn.WriteMessage(0, toWrite)
	}
}

func registerWs(cs *ConnectionStore, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	c := client{
		conn:      conn,
		identity:  []byte{},
		send:      make(chan []byte, 256),
		readOther: make(chan []byte, 256),
		store:     cs,
	}

	cs.register <- &c

	go c.read()
	go c.write()
}
