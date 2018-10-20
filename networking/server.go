package networking

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"sync"
	"time"

	"gopkg.in/dedis/kyber.v2"

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
	nShard          = 5
)

// ConnectionStore handles peer messaging
type ConnectionStore struct {
	clients   map[*client]bool
	broadcast chan []byte

	register   chan *client
	unregister chan *client

	beaconChain *blockchain.BeaconChain
	shardsChain map[uint32]*blockchain.Blockchain

	identity  *wallet.Wallet
	network   string
	interests map[string]bool

	// This is a very ugly hack to make it easy to delete clients
	interestedClients map[string]map[*client]bool
}

type client struct {
	conn      *websocket.Conn
	send      chan []byte
	readOther chan []byte
	store     *ConnectionStore
	wg        sync.WaitGroup
	interest  []string
	isOpen    bool
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Loop start ValidatorLoop for every interest
func (cs *ConnectionStore) Loop() {
	count := 0
	for interest := range cs.interests {
		interestInt, err := strconv.Atoi(interest)
		if err != nil {
			log.Error(err)
			continue
		}
		if interestInt == 0 {
			continue
		}
		count++

		// TODO UpdateChain
		// log.Info("Staring chain import")
		// cs.UpdateChain(uint32(interestInt))
		// log.Info("Done importing")

		// -1 because "0" doesn't count
		if len(cs.interests)-1 == count {
			cs.ValidatorLoop(uint32(interestInt))
		}
		go cs.ValidatorLoop(uint32(interestInt))
	}
}

// StartServer creates a new ConnectionStore, which handles network peers
func StartServer(port, network string, shardsChain map[uint32]*blockchain.Blockchain, beaconChain *blockchain.BeaconChain, idn *wallet.Wallet) (*ConnectionStore, error) {
	store := &ConnectionStore{
		clients:           make(map[*client]bool),
		broadcast:         make(chan []byte),
		register:          make(chan *client),
		unregister:        make(chan *client),
		beaconChain:       beaconChain,
		shardsChain:       shardsChain,
		identity:          idn,
		network:           network,
		interests:         make(map[string]bool),
		interestedClients: make(map[string]map[*client]bool),
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
			isOpen:    true,
		}

		store.register <- &c

		go c.read()
		go c.write()
	})

	// Server that allows peers to connect
	go http.ListenAndServe(port, nil)

	return store, nil
}

// AddInterest adds an interest, this is used as a filter to have more efficient
// broadcasts and avoid sending everything to everyone
func (cs *ConnectionStore) AddInterest(key string) {
	cs.interests[key] = true
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
		isOpen:    true,
	}

	cs.register <- &c

	keys := []string{}
	for k := range cs.interests {
		keys = append(keys, k)
	}
	p := &network.Interests{
		Keys: keys,
	}
	d, _ := proto.Marshal(p)
	e := &network.Envelope{
		Type: network.Envelope_INTERESTS,
		Data: d,
	}
	ed, _ := proto.Marshal(e)
	if c.isOpen {
		c.send <- ed
	}

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
			// Ask the connected client for the interests he has
			client.wg.Add(1)

			keys := []string{}

			for k := range cs.interests {
				keys = append(keys, k)
			}

			p := &network.Interests{
				Keys: keys,
			}

			d, _ := proto.Marshal(p)

			e := &network.Envelope{
				Type: network.Envelope_INTERESTS,
				Data: d,
			}

			ed, _ := proto.Marshal(e)

			if client.isOpen {
				client.send <- ed
			}

			// Unlock the send channel so we can kill the goroutine
			client.wg.Done()

			cs.clients[client] = true

		// A client has quit, check if it exisited and delete it
		case client := <-cs.unregister:
			if _, ok := cs.clients[client]; ok {
				// Don't close the channel till we're done responding to avoid panics
				client.wg.Wait()

				delete(cs.clients, client)

				// Delete the client from all his interests
				for _, v := range client.interest {
					delete(cs.interestedClients[v], client)
				}

				close(client.send)
			}

		// Network wide broadcast. For now this uses a very simple and broken
		// algorithm but it could be optimized using ASNs as an overlay network
		case message := <-cs.broadcast:
			env := &network.Envelope{}
			broadcastEnvelope := &network.Broadcast{}
			proto.Unmarshal(message, env)
			proto.Unmarshal(env.Data, broadcastEnvelope)

			shard := env.GetShard()

			broadcastEnvelope.TTL--
			if broadcastEnvelope.TTL < 1 || broadcastEnvelope.TTL > 64 {
				continue
			}

			broadcastBytes, err := proto.Marshal(broadcastEnvelope)
			if err != nil {
				log.Error(err)
				continue
			}
			newEnv := &protoNetwork.Envelope{
				Type:  protoNetwork.Envelope_BROADCAST,
				Data:  broadcastBytes,
				Shard: shard,
			}
			dataByte, err := proto.Marshal(newEnv)
			if err != nil {
				log.Error(err)
				continue
			}

			// send to everyone if there are a few clients
			if len(cs.clients) < 50 {
				for k := range cs.clients {
					if k.isOpen {
						k.send <- dataByte
					}
				}
			} else {
				// send the message to the interest client
				for k := range cs.interestedClients[fmt.Sprint(shard)] {
					if k.isOpen {
						k.send <- dataByte
					}
				}
			}
		}
	}
}

func checkDuplicatedMessage(msg []byte) bool {
	env := &network.Envelope{}
	broadcast := &network.Broadcast{}
	proto.Unmarshal(msg, env)
	proto.Unmarshal(env.Data, broadcast)

	// check signature
	bhash := sha256.Sum256(broadcast.GetData())
	hash := bhash[:]
	identityBroadcast := broadcast.GetIdentity()
	signatureValid, err := wallet.SignatureValid(identityBroadcast.GetPubkey(), identityBroadcast.GetR(), identityBroadcast.GetS(), hash)
	if !signatureValid || err != nil {
		log.Error("Fail signatureValid broadcast ", err)
		return false
	}

	// set TTL to 0, calculate the hash of the message, check if already exist
	copyBroadcast := *broadcast
	copyBroadcast.TTL = 0
	bhash = sha256.Sum256([]byte(fmt.Sprintf("%v", copyBroadcast)))
	hash = bhash[:]
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
		c.isOpen = false
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

			broadcastEnvelope := &protoNetwork.Broadcast{}
			err := proto.Unmarshal(pb.GetData(), broadcastEnvelope)
			if err != nil {
				continue
			}
			signature := broadcastEnvelope.GetIdentity()
			bhash := sha256.Sum256(pb.GetData())
			hashData := bhash[:]
			if !reflect.DeepEqual(hashData, signature.GetData()) {
				log.Error("Hash of the data doesn't match with the hash of data inside the signature")
				continue
			}
			valid, err := wallet.SignatureValid(signature.GetPubkey(), signature.GetR(), signature.GetS(), hashData)
			if !valid {
				log.Error("The signature of the message is invalid")
				continue
			}

			// TODO dentro ogni messaggio controllo se signature.GetPubkey() corrisponde a pb.GetData().GetPubkey()

			go c.store.handleBroadcast(pb.GetData(), pb.GetShard(), signature.GetPubkey())

			// check if the interest exist in interestedClients
			if _, ok := c.store.interestedClients[fmt.Sprint(pb.GetShard())]; ok {
				c.store.broadcast <- msg
			}

		// If the ContentType is a request then try to parse it as such and handle it
		case protoNetwork.Envelope_REQUEST:
			request := protoNetwork.Request{}
			err = proto.Unmarshal(pb.GetData(), &request)
			if err != nil {
				log.Error(err)
				continue
			}

			// Free up the goroutine to recive multi part messages
			go func() {
				// Increment the waitgroup to avoid panics
				c.wg.Add(1)
				rawMsg := c.store.handleMessage(&request, c, pb.GetShard())
				env := protoNetwork.Envelope{
					Type:  protoNetwork.Envelope_OTHER,
					Data:  rawMsg,
					Shard: pb.GetShard(),
				}

				toSend, err := proto.Marshal(&env)
				if err != nil {
					log.Error(err)
					return
				}

				if c.isOpen {
					c.send <- toSend
				}
				// Once we are done using the channel decrement the group
				c.wg.Done()
			}()

		// Other data can be channeled so other parts of code can use it
		case protoNetwork.Envelope_OTHER:
			c.readOther <- pb.GetData()

		case protoNetwork.Envelope_INTERESTS:
			intr := &protoNetwork.Interests{}

			err := proto.Unmarshal(pb.Data, intr)
			if err != nil {
				continue
			}

			c.interest = []string{}
			// Save the interests of the clients
			for _, v := range intr.Keys {
				c.interest = append(c.interest, v)
				if _, ok := c.store.interestedClients[v]; !ok {
					c.store.interestedClients[v] = make(map[*client]bool)
				}
				c.store.interestedClients[v][c] = true
			}

		case protoNetwork.Envelope_NEIGHBOUR_INTERESTS:
			peers := &protoNetwork.PeersAndInterests{}

			// create a connection with the peers that my neighbour know
			for _, i := range peers.GetIps() {
				c.store.Connect(i)
			}

			// also save the interest that send this message
			c.interest = []string{}
			for _, v := range peers.GetKeys() {
				c.interest = append(c.interest, v)
				if _, ok := c.store.interestedClients[v]; !ok {
					c.store.interestedClients[v] = make(map[*client]bool)
				}
				c.store.interestedClients[v][c] = true
			}

		}
	}
}

// write checks the channel for data to write and writes it to the socket
func (c *client) write() {
	for {
		toWrite := <-c.send

		_ = c.conn.WriteMessage(websocket.BinaryMessage, toWrite)
	}
}

// ValidatorLoop updates the current expected validator and generates a block
// if the validator has the same identity as the node generates a block
func (cs *ConnectionStore) ValidatorLoop(currentShard uint32) {
	if int64(cs.shardsChain[currentShard].GenesisTimestamp) > time.Now().Unix() {
		log.Info("Waiting for genesis")
		time.Sleep(time.Duration(int64(cs.shardsChain[currentShard].GenesisTimestamp)-time.Now().Unix()) * time.Second)
	}

	wal, err := cs.identity.GetWallet()
	if err != nil {
		log.Fatal(err)
	}
	log.Info("myWallet ", wal)

	var currentK kyber.Scalar

	for {
		// The validator changes every time the unix timestamp is a multiple of 5
		sleepTime := 1 + 4 - time.Now().Unix()%5
		time.Sleep(time.Duration(sleepTime) * time.Second)

		// check if the block with index cs.shardsChain[currentShard].CurrentBlock have been saved, otherwise save an empty block
		_, err := cs.shardsChain[currentShard].GetBlock(cs.shardsChain[currentShard].CurrentBlock)
		if err != nil {
			prevBlockByte, err := cs.shardsChain[currentShard].GetBlock(cs.shardsChain[currentShard].CurrentBlock - 1)
			if err != nil {
				log.Error(err)
				continue
			}
			bhash := sha256.Sum256(prevBlockByte)

			emptyBlock := &protoBlockchain.Block{
				Index:    cs.shardsChain[currentShard].CurrentBlock,
				PrevHash: bhash[:],
				Shard:    currentShard,
			}

			err = cs.SaveBlock(emptyBlock, currentShard)
			if err != nil {
				log.Error(err)
			}
			err = cs.ImportBlock(emptyBlock, currentShard)
			if err != nil {
				log.Error(err)
			}
		}

		// increment the block number
		cs.shardsChain[currentShard].CurrentBlock++
		log.Info("Current block ", cs.shardsChain[currentShard].CurrentBlock, " in shard ", currentShard)

		// chose a validator based on stake
		validator, err := cs.beaconChain.Validators.ChooseValidator(int64(cs.shardsChain[currentShard].CurrentBlock), currentShard, cs.shardsChain[currentShard])
		if err != nil {
			log.Error(err)
			continue
		}

		// Start accepting the block from the new validator
		cs.shardsChain[currentShard].CurrentValidator[cs.shardsChain[currentShard].CurrentBlock] = validator

		// after max 100 rounds send all your list of ips to every client that you know
		if int(rand.Float64()*100) > 100-int(cs.shardsChain[currentShard].CurrentBlock%100) {
			ips := []string{}
			for k := range cs.clients {
				ips = append(ips, k.conn.RemoteAddr().String())
			}

			keys := []string{}
			for k := range cs.interests {
				keys = append(keys, k)
			}

			peers := &network.PeersAndInterests{
				Keys: keys,
				Ips:  ips,
			}
			peersByte, _ := proto.Marshal(peers)
			env := &network.Envelope{
				Type:  network.Envelope_NEIGHBOUR_INTERESTS,
				Data:  peersByte,
				Shard: 0,
			}
			data, _ := proto.Marshal(env)

			for k := range cs.clients {
				k.send <- data
			}
		}

		// Change shard
		if cs.shardsChain[currentShard].CurrentBlock%10000 == 0 {
			// calulate the hash of the previous 100 blocks from current block - 1
			var hashBlocks []byte
			latestBlock := true
			for i := cs.shardsChain[currentShard].CurrentBlock - 1; i > cs.shardsChain[currentShard].CurrentBlock-100; i-- {
				currentBlockByte, err := cs.shardsChain[currentShard].GetBlock(i)
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
			newShard, err := cs.beaconChain.Validators.ChooseShard(int64(seed), wal, cs.shardsChain[currentShard])
			if err != nil {
				log.Error(err)
			}

			// TODO like this doesn't work, you shouldn't remove the blockchain so early

			// remove the older blockchain and create a new one
			os.RemoveAll(".dexm/shard")
			os.MkdirAll(".dexm/shard", os.ModePerm)
			cs.shardsChain[currentShard], err = blockchain.NewBlockchain(".dexm/shard/", 0)
			if err != nil {
				log.Fatal("NewBlockchain ", err)
			}
			// ask for the chain that corrispond to newShard shard
			cs.UpdateChain(newShard)
		}

		// check if you are a validator or not, if not don't continue with the other messages
		if !cs.beaconChain.Validators.CheckIsValidator(wal) {
			log.Info(wal, " is not a validator, so continue")
			continue
		}

		// every 30 blocks do the merkle root signature
		if cs.shardsChain[currentShard].CurrentBlock%30 == 0 {
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

			pub, _ := cs.identity.GetPubKey()
			bhash := sha256.Sum256(schnorrPByte)
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

			// broadcast schnorr message
			broadcastSchnorr := &network.Broadcast{
				Type:     protoNetwork.Broadcast_SCHNORR,
				TTL:      64,
				Identity: signature,
				Data:     schnorrPByte,
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
		if cs.shardsChain[currentShard].CurrentBlock%30 == 3 && cs.shardsChain[currentShard].CurrentBlock != 3 {
			// get the private schnorr key
			x, err := cs.identity.GetPrivateKeySchnorr()
			if err != nil {
				log.Error(err)
			}

			var myR []byte
			var myP []byte
			var Rs []kyber.Point
			var Ps []kyber.Point

			for key, value := range cs.shardsChain[currentShard].Schnorr {
				r, err := wallet.ByteToPoint(value)
				if err != nil {
					log.Error("r ByteToPoint ", err)
					continue
				}
				// i should't include myself in the sign, so i save it in difference variable
				if key == wal {
					myR, err = r.MarshalBinary()
					if err != nil {
						log.Error("r marshal ", err)
						continue
					}
					p, err := cs.identity.GetPublicKeySchnorr()
					if err != nil {
						log.Error("GetSchnorrPublicKey ", err)
						continue
					}
					myP, err = p.MarshalBinary()
					if err != nil {
						log.Error("p marshal ", err)
						continue
					}
					continue
				}
				p, err := cs.beaconChain.Validators.GetSchnorrPublicKey(key)
				if err != nil {
					log.Error("GetSchnorrPublicKey ", err)
					continue
				}
				Rs = append(Rs, r)
				Ps = append(Ps, p)
			}

			var MRReceipt []byte
			// -3 becuase it's after 3 turn from sending GenerateParameter
			for i := int64(cs.shardsChain[currentShard].CurrentBlock) - 3; i > int64(cs.shardsChain[currentShard].CurrentBlock)-33; i-- {
				blockByte, err := cs.shardsChain[currentShard].GetBlock(uint64(i))
				if err != nil {
					log.Error(err)
					continue
				}
				block := &protoBlockchain.Block{}
				proto.Unmarshal(blockByte, block)

				merkleRootReceipt := block.GetMerkleRootReceipt()
				MRReceipt = append(MRReceipt, merkleRootReceipt...)
			}

			// make the signature of the merkle roots transactions and receipts
			signReceipt := wallet.MakeSign(x, currentK, string(MRReceipt), Rs, Ps)

			// send the signed transaction and receipt
			signSchorrP := &protoBlockchain.SignSchnorr{
				Wallet:             wal,
				RSchnorr:           myR,
				PSchnorr:           myP,
				SignReceipt:        signReceipt,
				MessageSignReceipt: MRReceipt,
			}
			signSchorrByte, _ := proto.Marshal(signSchorrP)

			pub, _ := cs.identity.GetPubKey()
			bhash := sha256.Sum256(signSchorrByte)
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

			broadcastSignSchorr := &network.Broadcast{
				Type:     protoNetwork.Broadcast_SIGN_SCHNORR,
				TTL:      64,
				Identity: signature,
				Data:     signSchorrByte,
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
		if cs.shardsChain[currentShard].CurrentBlock%30 == 6 && cs.shardsChain[currentShard].CurrentBlock != 6 {
			var Rs []kyber.Point
			var Ps []kyber.Point
			var SsReceipt []kyber.Scalar

			// from all validator choosen get R, P and the signature of transaction and receipt
			for i := 0; i < len(cs.shardsChain[currentShard].RSchnorr); i++ {
				r, err := wallet.ByteToPoint(cs.shardsChain[currentShard].RSchnorr[i])
				if err != nil {
					log.Error(err)
					continue
				}
				p, err := wallet.ByteToPoint(cs.shardsChain[currentShard].PSchnorr[i])
				if err != nil {
					log.Error(err)
					continue
				}

				sReceipt, err := wallet.ByteToScalar(cs.shardsChain[currentShard].MTReceipt[i])
				if err != nil {
					log.Error(err)
					continue
				}

				Rs = append(Rs, r)
				Ps = append(Ps, p)
				SsReceipt = append(SsReceipt, sReceipt)
			}

			if len(Rs) == 0 || len(SsReceipt) == 0 {
				log.Error("Error length ", Rs, SsReceipt)
			} else {
				// generate the final signature
				RsignatureReceipt, SsignatureReceipt, err := wallet.CreateSignature(Rs, SsReceipt)
				if err != nil {
					log.Error(err)
				}

				// send the final signature
				mr := &protoBlockchain.MerkleRootsSigned{
					Shard:                     currentShard,
					MerkleRootsReceipt:        cs.shardsChain[currentShard].MessagesReceipt,
					RSignedMerkleRootsReceipt: RsignatureReceipt,
					SSignedMerkleRootsReceipt: SsignatureReceipt,
					RValidators:               cs.shardsChain[currentShard].RSchnorr,
					PValidators:               cs.shardsChain[currentShard].PSchnorr,
				}
				mrByte, _ := proto.Marshal(mr)

				pub, _ := cs.identity.GetPubKey()
				bhash := sha256.Sum256(mrByte)
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

				broadcastMr := &network.Broadcast{
					Type:     protoNetwork.Broadcast_MERKLE_ROOTS_SIGNED,
					TTL:      64,
					Identity: signature,
					Data:     mrByte,
				}
				broadcastMrByte, _ := proto.Marshal(broadcastMr)

				env := &network.Envelope{
					Type:  network.Envelope_BROADCAST,
					Data:  broadcastMrByte,
					Shard: 0,
				}

				data, _ := proto.Marshal(env)
				cs.broadcast <- data

				// send merkle proof of the other shard
				for i := int64(cs.shardsChain[currentShard].CurrentBlock) - 7; i > int64(cs.shardsChain[currentShard].CurrentBlock)-37; i-- {
					blockByte, err := cs.shardsChain[currentShard].GetBlock(uint64(i))
					if err != nil {
						log.Error(err)
						continue
					}
					block := &protoBlockchain.Block{}
					proto.Unmarshal(blockByte, block)

					// generate and send the merkle proof
					var receipts []*protoBlockchain.Receipt
					var transactions []*protoBlockchain.Transaction
					for _, t := range block.GetTransactions() {
						if t.GetShard() != currentShard {
							log.Info("not your shard to merkleproof")
							continue
						}
						r := &protoBlockchain.Receipt{
							Sender:    wallet.BytesToAddress(t.GetSender(), t.GetShard()),
							Recipient: t.GetRecipient(),
							Amount:    t.GetAmount(),
							Nonce:     t.GetNonce(),
						}
						receipts = append(receipts, r)
						transactions = append(transactions, t)
					}

					for i, _ := range receipts {
						log.Info("Receipts to prove")
						merkleProofByte := GenerateMerkleProof(receipts, i, transactions[i])
						if len(merkleProofByte) == 0 {
							log.Error("proof failed")
							continue
						}

						pub, _ := cs.identity.GetPubKey()
						bhash := sha256.Sum256(merkleProofByte)
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

						broadcastMerkleProof := &network.Broadcast{
							Type:     protoNetwork.Broadcast_MERKLE_PROOF,
							TTL:      64,
							Data:     merkleProofByte,
							Identity: signature,
						}
						broadcastMerkleProofByteByte, _ := proto.Marshal(broadcastMerkleProof)

						envMerkleProof := &network.Envelope{
							Type:  network.Envelope_BROADCAST,
							Data:  broadcastMerkleProofByteByte,
							Shard: 0,
						}

						dataMerkleProof, _ := proto.Marshal(envMerkleProof)
						cs.broadcast <- dataMerkleProof

						log.Info("Broadcast_MERKLE_PROOF sent")
					}
				}
			}

			// reset everything about schnorr for the next message
			// for k := range cs.beaconChain.CurrentSign {
			// 	delete(cs.beaconChain.CurrentSign, k)
			// }
			for k := range cs.shardsChain[currentShard].Schnorr {
				delete(cs.shardsChain[currentShard].Schnorr, k)
			}
			cs.shardsChain[currentShard].MTReceipt = [][]byte{}
			cs.shardsChain[currentShard].RSchnorr = [][]byte{}
			cs.shardsChain[currentShard].PSchnorr = [][]byte{}
			cs.shardsChain[currentShard].MessagesReceipt = [][]byte{}
		}

		// Checkpoint Agreement
		if cs.shardsChain[currentShard].CurrentBlock%100 == 0 && cs.shardsChain[currentShard].CurrentBlock%10000 != 0 {
			// check if it is a validator, also check that the dynasty are correct
			if cs.beaconChain.Validators.CheckDynasty(wal, cs.shardsChain[currentShard].CurrentBlock) {
				// get source and target block in the blockchain
				souceBlockByte, err := cs.shardsChain[currentShard].GetBlock(cs.shardsChain[currentShard].CurrentCheckpoint)
				if err != nil {
					log.Error("Get block ", err)
					continue
				}
				targetBlockByte, err := cs.shardsChain[currentShard].GetBlock(cs.shardsChain[currentShard].CurrentBlock - 1)
				if err != nil {
					log.Error("Get block ", err)
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

				// create the casper vote for the agreement of checkpoint
				vote := CreateVote(hashSource, hashTarget, cs.shardsChain[currentShard].CurrentCheckpoint, cs.shardsChain[currentShard].CurrentBlock-1, cs.identity)

				// read all the incoming vote and store it, after 15 second call CheckpointAgreement
				go func() {
					currentBlockCheckpoint := cs.shardsChain[currentShard].CurrentBlock - 1
					time.Sleep(15 * time.Second)
					check := cs.CheckpointAgreement(cs.shardsChain[currentShard].CurrentCheckpoint, currentBlockCheckpoint, currentShard)
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

		// If this node is the validator then generate a block and sign it
		if wal == validator {
			log.Info("I'm the miner ", wal)
			block, err := cs.shardsChain[currentShard].GenerateBlock(wal, currentShard, cs.beaconChain.Validators)
			if err != nil {
				log.Error(err)
				continue
			}

			err = cs.SaveBlock(block, currentShard)
			if err != nil {
				log.Error(err)
				continue
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
			log.Info("Block generated")
		}
	}
}
