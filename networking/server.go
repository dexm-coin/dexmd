package networking

import (
	"net/http"

	"github.com/dexm-coin/dexmd/blockchain"
	protobufs "github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
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
func StartServer(port string) (*ConnectionStore, error) {
	store := &ConnectionStore{
		clients:    make(map[*client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *client),
		unregister: make(chan *client),
	}

	go store.run()

	return store, nil
}

// run is the event handler to update the ConnectionStore
func (cs *ConnectionStore) run() {
	for {
		select {
		// A new client has registered
		case client := <-cs.register:
			cs.clients[client] = true

		// A client has quit, check if it exsited and delete it
		case client := <-cs.unregister:
			if _, ok := cs.clients[client]; ok {
				delete(cs.clients, client)
				close(client.send)
			}
		}
	}
}

// read reads data from the socket and handles it
func (c *client) read(bc blockchain.Blockchain) {

	// Unregister if the node dies
	defer func() {
		c.store.unregister <- c
		c.conn.Close()
	}()

	for {
		_, msg, _ := c.conn.ReadMessage() // TODO Handle go away messages

		pb := protobufs.Envelope{}
		err := proto.Unmarshal(msg, &pb)

		if err != nil {
			continue
		}

		switch pb.GetType() {

		// If the ContentType is a request then try to parse it as such and handle it
		case protobufs.Envelope_REQUEST:
			request := protobufs.Request{}
			err := proto.Unmarshal(msg, &request)
			if err != nil {
				continue
			}
			handleMessage(bc, &request)

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

func registerWs(bc blockchain.Blockchain, cs *ConnectionStore, w http.ResponseWriter, r *http.Request) {
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

	go c.read(bc)
	go c.write()
}
