package networking

import (
	"net/http"

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

	http.ListenAndServe(port, nil)

	return store, nil
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

		// TODO Verify validity of message inside broadcast
		case protobufs.Envelope_BROADCAST:
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
