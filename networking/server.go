package networking

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
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
	maxMessagesSave = 5000
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
	// wallet    string
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Loop start ValidatorLoop for every interest
func (cs *ConnectionStore) Loop() {
	var interests []uint32
	for interest := range cs.interests {
		interestInt, err := strconv.Atoi(interest)
		if err != nil {
			log.Error(err)
			continue
		}
		if interestInt == 0 {
			continue
		}
		interests = append(interests, uint32(interestInt))
	}

	for i, interest := range interests {
		log.Info("Staring chain import on shard ", interest)
		cs.UpdateChain(interest, "")
		log.Info("Done importing on shard ", interest)

		// don't "go" the last interest
		if i == len(interests)-1 {
			cs.ValidatorLoop(interest)
		}
		go cs.ValidatorLoop(interest)
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
			log.Error(err)
			return
		}

		c := client{
			conn:      conn,
			send:      make(chan []byte, 256),
			readOther: make(chan []byte, 256),
			store:     store,
			isOpen:    true,
		}

		// ts := uint64(time.Now().Unix())
		// byteTs := make([]byte, 4)
		// binary.LittleEndian.PutUint64(byteTs, ts)

		// // TODOMaybe quando qualcuno si connette gli chiedo subito il suo wallet
		// err = MakeSpecificRequest(0, byteTs, protoNetwork.Request_GET_WALLET, conn)
		// if err != nil {
		// 	log.Error(err)
		// 	return
		// }

		// signatureByte, err := c.GetResponse(100 * time.Millisecond)
		// if err != nil {
		// 	log.Error(err)
		// 	return
		// }

		// signature := &protoNetwork.Signature{}
		// err = proto.Unmarshal(signatureByte, signature)
		// if err != nil {
		// 	log.Error(err)
		// 	return
		// }
		// walletSender := wallet.BytesToAddress(signature.GetPubkey(), signature.GetShard())
		// if !wallet.IsWalletValid(walletSender) {
		// 	log.Error("Not a valid wallet")
		// 	return
		// }

		// for _, wallet := range store.clients {
		// 	if wallet == walletSender {
		// 		log.Error("Wallet already register")
		// 		return
		// 	}
		// }

		// msg := &protoNetwork.RandomMessage{}

		// c.wallet =

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
func (cs *ConnectionStore) RemoveInterest(key string) {
	if _, ok := cs.interests[key]; ok {
		delete(cs.interests, key)
	}
}

func (cs *ConnectionStore) AddGenesisToQueue(block *protoBlockchain.Block, shard uint32) {
	res, _ := proto.Marshal(block)
	bhash := sha256.Sum256(res)
	hash := bhash[:]

	h := sha256.New()
	h.Write(hash)
	hashBlock := hex.EncodeToString(h.Sum(nil))

	cs.shardsChain[shard].PriorityBlocks.Insert(hashBlock, 0)
	cs.shardsChain[shard].HashBlocks[hashBlock] = 0
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

	pubKey, _ := cs.identity.GetPubKey()

	keys := []string{}
	for k := range cs.interests {
		keys = append(keys, k)
	}

	p := &network.Interests{
		Keys:         keys,
		Address:      pubKey,
		ShardAddress: uint32(cs.identity.GetShardWallet()),
	}
	d, _ := proto.Marshal(p)
	bhashInterest := sha256.Sum256(d)
	hashInterest := bhashInterest[:]

	// Sign the new block
	r, s, err := cs.identity.Sign(hashInterest)
	if err != nil {
		log.Error(err)
	}

	signature := &network.Signature{
		Pubkey: pubKey,
		R:      r.Bytes(),
		S:      s.Bytes(),
		Data:   hashInterest,
	}

	env := &network.Envelope{
		Data:     d,
		Type:     network.Envelope_INTERESTS,
		Shard:    0,
		Identity: signature,
	}
	data, _ := proto.Marshal(env)

	if c.isOpen {
		c.send <- data
	}

	go c.read()
	go c.write()

	return nil
}

// save the 100's hash of newest messages (without the ttl) arrived
var hashMessages = make([][]byte, 0)

// run is the event handler to update the ConnectionStore
func (cs *ConnectionStore) run() {
	for {
		select {
		// A new client has registered
		case client := <-cs.register:
			// Ask the connected client for the interests he has
			client.wg.Add(1)

			pubKey, _ := cs.identity.GetPubKey()

			keys := []string{}
			for k := range cs.interests {
				keys = append(keys, k)
			}

			p := &network.Interests{
				Keys:         keys,
				Address:      pubKey,
				ShardAddress: uint32(cs.identity.GetShardWallet()),
			}
			d, _ := proto.Marshal(p)
			bhashInterest := sha256.Sum256(d)
			hashInterest := bhashInterest[:]

			// Sign the new block
			r, s, err := cs.identity.Sign(hashInterest)
			if err != nil {
				log.Error(err)
			}

			signature := &network.Signature{
				Pubkey: pubKey,
				R:      r.Bytes(),
				S:      s.Bytes(),
				Data:   hashInterest,
			}

			env := &network.Envelope{
				Data:     d,
				Type:     network.Envelope_INTERESTS,
				Shard:    0,
				Identity: signature,
			}
			data, _ := proto.Marshal(env)

			if client.isOpen {
				client.send <- data
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
			proto.Unmarshal(message, env)

			bhash := sha256.Sum256(env.GetData())
			hash := bhash[:]
			identity := env.GetIdentity()
			signatureValid, err := wallet.SignatureValid(identity.GetPubkey(), identity.GetR(), identity.GetS(), hash)
			if !signatureValid || err != nil {
				log.Error("Fail signatureValid ", err)
				continue
			}
			if !reflect.DeepEqual(hash, identity.GetData()) {
				log.Error("Hash of the data doesn't match with the hash of data inside the signature")
				continue
			}

			shard := env.GetShard()

			env.TTL--
			if env.TTL < 1 || env.TTL > 64 {
				continue
			}

			dataByte, err := proto.Marshal(env)
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

func checkDuplicatedMessage(hash []byte) bool {
	alreadyReceived := false
	for _, h := range hashMessages {
		equal := reflect.DeepEqual(h, hash)
		if equal {
			alreadyReceived = true
			break
		}
	}
	if alreadyReceived {
		// skip the message
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
		// log.Info("Client died")
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

		bhash := sha256.Sum256(pb.GetData())
		hash := bhash[:]
		identity := pb.GetIdentity()
		signatureValid, err := wallet.SignatureValid(identity.GetPubkey(), identity.GetR(), identity.GetS(), hash)
		if !signatureValid || err != nil {
			log.Error("Fail signatureValid ", err)
			continue
		}
		if !reflect.DeepEqual(hash, identity.GetData()) {
			log.Error("Hash of the data doesn't match with the hash of data inside the signature")
			continue
		}

		switch pb.GetType() {
		case protoNetwork.Envelope_BROADCAST:
			skip := checkDuplicatedMessage(hash)
			if skip {
				continue
			}

			go c.store.handleBroadcast(pb.GetData(), pb.GetShard(), identity)

			// check if the interest exist in interestedClients
			if _, ok := c.store.interestedClients[fmt.Sprint(pb.GetShard())]; ok {
				c.store.broadcast <- msg
			}

		// If the ContentType is a request then try to parse it as such and handle it
		case protoNetwork.Envelope_REQUEST:
			// Free up the goroutine to recive multi part messages
			go func() {
				// Increment the waitgroup to avoid panics
				c.wg.Add(1)

				rawMsg := c.store.handleMessage(pb.GetData(), c, pb.GetShard(), identity)

				pubKey, _ := c.store.identity.GetPubKey()

				bhashBroadcast := sha256.Sum256(rawMsg)
				hashBroadcast := bhashBroadcast[:]

				r, s, err := c.store.identity.Sign(hashBroadcast)
				if err != nil {
					log.Error(err)
				}

				signature := &network.Signature{
					Pubkey: pubKey,
					R:      r.Bytes(),
					S:      s.Bytes(),
					Data:   hashBroadcast,
				}

				env := &network.Envelope{
					Data:     rawMsg,
					Type:     protoNetwork.Envelope_OTHER,
					Shard:    pb.GetShard(),
					Identity: signature,
				}
				data, _ := proto.Marshal(env)

				if c.isOpen {
					c.send <- data
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
		sleepTime := 2 + time.Now().Unix()%5
		time.Sleep(time.Duration(sleepTime) * time.Second)

		// // check if the block with index cs.shardsChain[currentShard].CurrentBlock have been saved, otherwise save an empty block
		// _, err := cs.shardsChain[currentShard].GetBlock(cs.shardsChain[currentShard].CurrentBlock)
		// if err != nil {
		// 	prevBlockByte, err := cs.shardsChain[currentShard].GetBlock(cs.shardsChain[currentShard].CurrentBlock - 1)
		// 	if err != nil {
		// 		log.Error(err)
		// 		continue
		// 	}
		// 	bhash := sha256.Sum256(prevBlockByte)

		// 	emptyBlock := &protoBlockchain.Block{
		// 		Index:    cs.shardsChain[currentShard].CurrentBlock,
		// 		PrevHash: bhash[:],
		// 		Shard:    currentShard,
		// 	}

		// 	err = cs.SaveBlock(emptyBlock, currentShard)
		// 	if err != nil {
		// 		log.Error(err)
		// 	}
		// 	err = cs.ImportBlock(emptyBlock, currentShard)
		// 	if err != nil {
		// 		log.Error(err)
		// 	}
		// }

		// increment the block number
		cs.shardsChain[currentShard].CurrentBlock++

		// chose a validator based on stake
		validator, err := cs.beaconChain.Validators.ChooseValidator(int64(cs.shardsChain[currentShard].CurrentBlock), currentShard, cs.shardsChain[currentShard])
		if err != nil {
			log.Error(err)
			continue
		}
		log.Info("Block ", cs.shardsChain[currentShard].CurrentBlock, " in shard ", currentShard, " ChooseValidator ", validator)

		// Start accepting the block from the new validator
		cs.shardsChain[currentShard].CurrentValidator[cs.shardsChain[currentShard].CurrentBlock] = validator

		time.Sleep(time.Duration(1) * time.Second)

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

			pubKey, _ := cs.identity.GetPubKey()

			peers := &network.PeersAndInterests{
				Keys:         keys,
				Ips:          ips,
				Address:      pubKey,
				ShardAddress: uint32(cs.identity.GetShardWallet()),
			}
			peersByte, _ := proto.Marshal(peers)
			bhashInterest := sha256.Sum256(peersByte)
			hashInterest := bhashInterest[:]

			r, s, err := cs.identity.Sign(hashInterest)
			if err != nil {
				log.Error(err)
			}

			signature := &network.Signature{
				Pubkey: pubKey,
				R:      r.Bytes(),
				S:      s.Bytes(),
				Data:   hashInterest,
			}

			env := &network.Envelope{
				Data:     peersByte,
				Type:     network.Envelope_NEIGHBOUR_INTERESTS,
				Shard:    0,
				Identity: signature,
			}
			data, _ := proto.Marshal(env)

			for k := range cs.clients {
				k.send <- data
			}
		}

		// Change shard
		// TODO add || cs.shardsChain[currentShard].CurrentBlock == 1
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
			newShardStr := fmt.Sprint(newShard)

			// check if create or not the new blockchain in shard newShard
			if _, ok := cs.shardsChain[newShard]; ok {
				// check if the blockchain in shard newShard that i have, is full update
				if !(cs.shardsChain[newShard].CurrentBlock == cs.shardsChain[currentShard].CurrentBlock) {
					err := cs.UpdateChain(newShard, fmt.Sprint(currentShard))
					if err != nil {
						log.Fatal("UpdateChain ", err)
						continue
					}
				}
			} else {
				os.MkdirAll(".dexm/shard"+newShardStr+"/", os.ModePerm)
				cs.shardsChain[newShard], err = blockchain.NewBlockchain(".dexm/shard"+newShardStr+"/", 0)
				if err != nil {
					log.Fatal("NewBlockchain ", err)
					continue
				}
				// ask for the chain that corrispond to newShard shard
				err := cs.UpdateChain(newShard, fmt.Sprint(currentShard))
				if err != nil {
					log.Fatal("UpdateChain ", err)
					continue
				}
			}
		}

		// check if you are a validator or not, if not don't continue with the other messages
		if !cs.beaconChain.Validators.CheckIsValidator(wal) {
			log.Info(wal, " is not a validator, so continue")
			continue
		}

		// If this node is the validator then generate a block, sign it, and send the signature with the block
		if wal == validator {
			log.Info("I'm the miner ", wal)

			// hashOldBlock := []byte{}PriorityBlocks
			// bc := cs.shardsChain[currentShard]
			// index := bc.CurrentBlock
			// for {
			// 	index--
			// 	byteBlock, err := bc.GetBlock(index)
			// 	if err != nil {
			// 		// TODOMaybe fai request a tutti di quell'indice e controlla il match della signature sia del validator scelto
			// 		block, verify := cs.RequestMissingBlock(currentShard, index)
			// 		if verify && block != nil {
			// 			bhash := sha256.Sum256(currBlock)
			// 			hashOldBlock = bhash[:]
			// 			break
			// 		}
			// 	}
			// }

			block, err := cs.shardsChain[currentShard].GenerateBlock(wal, currentShard, cs.beaconChain.Validators)
			if err != nil {
				log.Error(err)
				continue
			}
			blockBytes, _ := proto.Marshal(block)

			bhash := sha256.Sum256(blockBytes)
			hash := bhash[:]

			prevHash := block.GetPrevHash()
			h := sha256.New()
			h.Write(prevHash)
			hashPrevBlockString := hex.EncodeToString(h.Sum(nil))
			h2 := sha256.New()
			h2.Write(hash)
			hashCurrentBlockString := hex.EncodeToString(h.Sum(nil))

			cs.shardsChain[currentShard].HashBlocks[hashCurrentBlockString] = cs.shardsChain[currentShard].HashBlocks[hashPrevBlockString] + 1
			cs.shardsChain[currentShard].PriorityBlocks.Insert(hashCurrentBlockString, float64(cs.shardsChain[currentShard].HashBlocks[hashCurrentBlockString])+0.5)

			cs.MakeEnvelopeBroadcast(blockBytes, network.Broadcast_BLOCK_PROPOSAL, 1, currentShard)
			log.Info("Block generated")
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

			cs.MakeEnvelopeBroadcast(schnorrPByte, protoNetwork.Broadcast_SCHNORR, 1, currentShard)
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
						log.Error("GetPublicKeySchnorr ", err)
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

			cs.MakeEnvelopeBroadcast(signSchorrByte, protoNetwork.Broadcast_SIGN_SCHNORR, 1, currentShard)
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

				cs.MakeEnvelopeBroadcast(mrByte, protoNetwork.Broadcast_MERKLE_ROOTS_SIGNED, 1, 0)

				// wait 10 second and send the merkle proof
				go func() {
					time.Sleep(10 * time.Second)

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
							merkleProofByte := GenerateMerkleProof(receipts, i, transactions[i])
							if len(merkleProofByte) == 0 {
								log.Error("proof failed")
								continue
							}

							cs.MakeEnvelopeBroadcast(merkleProofByte, protoNetwork.Broadcast_MERKLE_PROOF, 1, 0)
						}
					}
				}()

			}

			for k := range cs.shardsChain[currentShard].Schnorr {
				delete(cs.shardsChain[currentShard].Schnorr, k)
			}
			cs.shardsChain[currentShard].MTReceipt = [][]byte{}
			cs.shardsChain[currentShard].RSchnorr = [][]byte{}
			cs.shardsChain[currentShard].PSchnorr = [][]byte{}
			cs.shardsChain[currentShard].MessagesReceipt = [][]byte{}
		}

		// Checkpoint Agreement
		// if cs.shardsChain[currentShard].CurrentBlock%100 == 0 && cs.shardsChain[currentShard].CurrentBlock%10000 != 0 {
		// 	// check if it is a validator, also check that the dynasty are correct
		// 	if cs.beaconChain.Validators.CheckDynasty(wal, cs.shardsChain[currentShard].CurrentBlock) {
		// 		// get source and target block in the blockchain
		// 		souceBlockByte, err := cs.shardsChain[currentShard].GetBlock(cs.shardsChain[currentShard].CurrentCheckpoint)
		// 		if err != nil {
		// 			log.Error("Get block ", err)
		// 			continue
		// 		}
		// 		targetBlockByte, err := cs.shardsChain[currentShard].GetBlock(cs.shardsChain[currentShard].CurrentBlock - 1)
		// 		if err != nil {
		// 			log.Error("Get block ", err)
		// 			continue
		// 		}

		// 		source := &protoBlockchain.Block{}
		// 		target := &protoBlockchain.Block{}
		// 		proto.Unmarshal(souceBlockByte, source)
		// 		proto.Unmarshal(targetBlockByte, target)

		// 		bhash1 := sha256.Sum256(souceBlockByte)
		// 		hashSource := bhash1[:]
		// 		bhash2 := sha256.Sum256(targetBlockByte)
		// 		hashTarget := bhash2[:]

		// 		// create the casper vote for the agreement of checkpoint
		// 		vote := CreateVote(hashSource, hashTarget, cs.shardsChain[currentShard].CurrentCheckpoint, cs.shardsChain[currentShard].CurrentBlock-1, cs.identity)

		// 		// read all the incoming vote and store it, after 15 second call CheckpointAgreement
		// 		go func() {
		// 			currentBlockCheckpoint := cs.shardsChain[currentShard].CurrentBlock - 1
		// 			time.Sleep(15 * time.Second)
		// 			check := cs.CheckpointAgreement(cs.shardsChain[currentShard].CurrentCheckpoint, currentBlockCheckpoint, currentShard)
		// 			log.Info("CheckpointAgreement ", check)
		// 		}()

		// 		voteBytes, _ := proto.Marshal(&vote)
		// 		pub, _ := cs.identity.GetPubKey()
		// 		bhash := sha256.Sum256(voteBytes)
		// 		hash := bhash[:]

		// 		r, s, err := cs.identity.Sign(hash)
		// 		if err != nil {
		// 			log.Error(err)
		// 			continue
		// 		}
		// 		signature := &network.Signature{
		// 			Pubkey: pub,
		// 			R:      r.Bytes(),
		// 			S:      s.Bytes(),
		// 			Data:   hash,
		// 			Shard:  1,
		// 		}

		// 		broadcast := &network.Broadcast{
		// 			Data:     voteBytes,
		// 			Type:     network.Broadcast_CHECKPOINT_VOTE,
		// 			Identity: signature,
		// 			TTL:      64,
		// 		}
		// 		broadcastBytes, _ := proto.Marshal(broadcast)

		// 		env := &network.Envelope{
		// 			Type:  network.Envelope_BROADCAST,
		// 			Data:  broadcastBytes,
		// 			Shard: currentShard,
		// 		}

		// 		cs.MakeEnvelopeBroadcast(merkleProofByte, protoNetwork.Broadcast_MERKLE_PROOF, 1, 0)
		// 		cs.broadcast <- data
		// 	}
		// }

	}
}
