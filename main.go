package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/abiosoft/ishell"
	"github.com/dexm-coin/dexmd/networking"
	"github.com/gorilla/websocket"

	"github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/contracts"
	"github.com/dexm-coin/dexmd/wallet"
	bp "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/dexm-coin/protobufs/build/network"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const (
	PORT              = 3141
	PUBLIC_PEERSERVER = false
)

func main() {
	app := cli.NewApp()
	app.Version = "1.0.0 pre-alpha"
	app.Commands = []cli.Command{
		{
			Name:    "makewallet",
			Usage:   "mw [filename]",
			Aliases: []string{"genwallet", "mw", "gw"},
			Action: func(c *cli.Context) error {
				wal, _ := wallet.GenerateWallet()
				addr, _ := wal.GetWallet()
				log.Info("Generated wallet ", addr)

				if c.Args().Get(0) == "" {
					log.Fatal("Invalid filename")
				}

				wal.ExportWallet(c.Args().Get(0))

				return nil
			},
		},

		{
			Name:    "startnode",
			Usage:   "sn [wallet] [network]",
			Aliases: []string{"sn", "rn"},
			Action: func(c *cli.Context) error {
				walletPath := c.Args().Get(0)
				network := c.Args().Get(1)

				if network == "" {
					network = "hackney"
				}

				// Import an identity to encrypt data and sign for validator msg
				w, err := wallet.ImportWallet(walletPath)
				if err != nil {
					log.Fatal("import", err)
				}

				// Find the home folder of the current user
				// user, err := user.Current()
				// if err != nil {
				// 	log.Fatal("user", err)
				// }

				// Create the dexm folder in case it's not there
				os.MkdirAll(".dexm", os.ModePerm)

				// Create the blockchain database
				b, err := blockchain.NewBlockchain(".dexm", 0)
				if err != nil {
					log.Fatal("blockchain", err)
				}

				log.Info("Adding genesis block...")

				log.Info(time.Now().Unix())

				genesisBlock := &bp.Block{
					Index:     0,
					Timestamp: 1535753100,
					Miner:     "Dexm3ENiLVMNwaeRswEbV1PT7UEpDNwwlbef2e683",
				}
				b.SaveBlock(genesisBlock)
				b.ImportBlock(genesisBlock)

				// Open the port on the router, ignore errors
				networking.TraverseNat(PORT, "Dexm Blockchain Node")

				// Open a client on the default port
				cs, err := networking.StartServer(
					fmt.Sprintf(":%d", PORT),
					network,
					b,
					w,
				)
				if err != nil {
					log.Fatal("start", err)
				}

				// This is only supposed to be one for nodes that are
				// pointed to by *.dexm.space. Off by default
				if PUBLIC_PEERSERVER {
					log.Info("Staring public peerserver")
					go cs.StartPeerServer()
				}

				cs.FindPeers()

				// Update chain before
				log.Info("Staring chain import")

				// cs.UpdateChain()

				log.Info("Done importing")

				cs.ValidatorLoop()
				return nil
			},
		},

		{
			Name:    "maketransaction",
			Usage:   "mkt [walletPath] [recipient] [amount] [gas] [contract]",
			Aliases: []string{"mkt", "gt"},
			Action: func(c *cli.Context) error {
				// User supplied arguments
				walletPath := c.Args().Get(0)
				recipient := c.Args().Get(1)

				amount, err := strconv.ParseUint(c.Args().Get(2), 10, 64)
				if err != nil {
					log.Fatal(err)
				}

				gas, err := strconv.Atoi(c.Args().Get(3))
				if err != nil {
					log.Fatal(err)
				}

				cdata := []byte{}
				contractPath := c.Args().Get(4)

				if contractPath != "" {
					cdata, err = ioutil.ReadFile(contractPath)
					if err != nil {
						log.Fatal(err)
					}
				}

				senderWallet, err := wallet.ImportWallet(walletPath)
				if err != nil {
					log.Error("import", err)
					return nil
				}

				ips, err := networking.GetPeerList("hackney")
				if err != nil {
					log.Error("peer", err)
					return nil
				}

				dial := websocket.Dialer{
					Proxy:            http.ProxyFromEnvironment,
					HandshakeTimeout: 5 * time.Second,
				}

				log.Info(ips)
				for _, ip := range ips {
					conn, _, err := dial.Dial(fmt.Sprintf("ws://%s/ws", ip+":"+strconv.Itoa(PORT)), nil)
					if err != nil {
						log.Fatal(err)
						continue
					}

					req := &network.Request{
						Type: network.Request_GET_WALLET_STATUS,
					}

					reqD, _ := proto.Marshal(req)

					env := &network.Envelope{
						Type: network.Envelope_REQUEST,
						Data: reqD,
					}

					// GET_WALLET_STATUS requires to first send a request and then the address
					envD, _ := proto.Marshal(env)
					err = conn.WriteMessage(websocket.BinaryMessage, envD)
					if err != nil {
						log.Fatal(err)
						continue
					}

					senderAddr, _ := senderWallet.GetWallet()
					senderEnv := &network.Envelope{
						Type: network.Envelope_OTHER,
						Data: []byte(senderAddr),
					}

					senderAddrD, _ := proto.Marshal(senderEnv)

					err = conn.WriteMessage(websocket.BinaryMessage, []byte(senderAddrD))
					if err != nil {
						log.Fatal(err)
						continue
					}

					// Parse the message and save the new state
					_, msg, err := conn.ReadMessage()
					if err != nil {
						log.Fatal(err)
						continue
					}

					walletEnv := &network.Envelope{}
					err = proto.Unmarshal(msg, walletEnv)
					if err != nil {
						log.Fatal(err)
					}

					var walletStatus bp.AccountState
					err = proto.Unmarshal(walletEnv.Data, &walletStatus)
					if err != nil {
						log.Fatal(err)
					}
					log.Info("walletStatus")
					log.Info(walletStatus)
					senderWallet.Nonce = int(walletStatus.Nonce)
					senderWallet.Balance = int(walletStatus.Balance)

					trans, err := senderWallet.NewTransaction(recipient, amount, uint32(gas), cdata)
					if err != nil {
						log.Fatal(err)
						continue
					}

					// signature := &network.Signature{
					// 	Pubkey: pub,
					// 	R:      r.Bytes(),
					// 	S:      s.Bytes(),
					// 	Data:   hash,
					// }
					trBroad := &network.Broadcast{
						Type: network.Broadcast_TRANSACTION,
						Data: trans,
						// identity
						TTL: 64,
					}

					brD, _ := proto.Marshal(trBroad)

					trEnv := &network.Envelope{
						Type: network.Envelope_BROADCAST,
						Data: brD,
					}

					finalD, _ := proto.Marshal(trEnv)
					conn.WriteMessage(websocket.BinaryMessage, finalD)

					senderWallet.ExportWallet(walletPath)
					log.Info("Transaction done successfully")

					return nil
				}
				log.Error("Node not found")
				return nil
			},
		},

		{
			Name:    "interact",
			Usage:   "i [address]",
			Aliases: []string{"i"},
			Action: func(c *cli.Context) error {
				address := c.Args().Get(0)

				// Find the home folder of the current user
				// user, err := user.Current()
				// if err != nil {
				// 	log.Fatal(err)
				// }

				b, err := blockchain.NewBlockchain(".dexm", 0)
				if err != nil {
					log.Fatal(err)
					return nil
				}

				contract, err := contracts.GetContract(address, b.ContractDb, b.StateDb)
				if err != nil {
					return nil
				}

				shell := ishell.New()

				var entries []string
				for key := range contract.Module.Export.Entries {
					entries = append(entries, key)
				}

				var choice int
				shell.AddCmd(&ishell.Cmd{
					Name: "entries",
					Help: "fucntions entries from the contract",
					Func: func(c *ishell.Context) {
						choice = c.MultiChoice(entries, "Which function do you want to use ?")
					},
				})

				return nil
			},
		},

		{
			Name:    "makevanitywallet",
			Usage:   "mvw [filename] [regex] [cores]",
			Aliases: []string{"mvw", "mv"},
			Action: func(c *cli.Context) error {

				userWallet := c.Args().Get(0)
				vanity := c.Args().Get(1)
				cores, err := strconv.Atoi(c.Args().Get(2))

				if err != nil {
					log.Error(err)
					return nil
				}

				if c.Args().Get(0) == "" {
					log.Fatal("Invalid filename")
					return nil
				}

				if len(vanity) > 50 {
					log.Fatal("Regex too long")
					return nil
				}

				for _, letter := range vanity {
					correct := strings.Contains("123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz", string(letter))
					if !correct {
						log.Error("Dexm uses Base58 encoding, only chars allowed are 123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz")
						return nil
					}
				}

				vainityFound := false
				var wg sync.WaitGroup

				for i := 0; i < cores; i++ {
					wg.Add(1)
					go wallet.GenerateVanityWallet(vanity, userWallet, &vainityFound, &wg)
				}
				wg.Wait()

				return nil
			},
		},
	}

	app.Run(os.Args)
}
