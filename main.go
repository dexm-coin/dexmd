package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/networking"
	"github.com/dexm-coin/dexmd/wallet"
	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/dexm-coin/protobufs/build/network"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
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
			Usage:   "sn",
			Aliases: []string{"sn", "rn"},
			Action: func(c *cli.Context) error {
				port, _ := strconv.Atoi(c.Args().Get(0))
				if port == 8000 {
					log.Fatal("Invalid port")
				}

				bch, _ := blockchain.NewBlockchain(fmt.Sprintf("simulation/Blockchain_%d", port), 0)
				idn, err := wallet.ImportWallet(c.Args().Get(1))
				if err != nil {
					log.Fatal(err)
				}

				// Import the genesis block
				raw, err := ioutil.ReadFile("genesis.json")
				if err != nil {
					log.Fatal(err)
				}

				hexTransactions := []string{}
				json.Unmarshal(raw, &hexTransactions)

				var transactions []*protobufs.Transaction

				for _, t := range hexTransactions {
					tx := &protobufs.Transaction{}
					txBytes, err := hex.DecodeString(t)
					if err != nil {
						log.Fatal(err)
					}

					err = proto.Unmarshal(txBytes, tx)
					if err != nil {
						log.Fatal(err)
					}

					transactions = append(transactions, tx)
				}

				block := protobufs.Block{
					Index:        0,
					Timestamp:    uint64(time.Now().Unix()),
					PrevHash:     []byte{},
					Transactions: transactions,
				}

				bch.SaveBlock(&block)
				bch.ImportBlock(&block)

				networking.StartServer(fmt.Sprintf(":%d", port), bch, idn)

				return nil
			},
		},

		{
			Name:    "maketransaction",
			Usage:   "mkt [walletPath] [recipient] [amount]",
			Aliases: []string{"mkt", "gt"},
			Action: func(c *cli.Context) error {
				walletPath := c.Args().Get(0)
				recipient := c.Args().Get(1)
				amount, err := strconv.ParseUint(c.Args().Get(2), 10, 64)

				if err != nil {
					log.Error(err)
					return nil
				}
				senderWallet, err := wallet.ImportWallet(walletPath)
				if err != nil {
					log.Error(err)
					return nil
				}

				transaction, err := senderWallet.NewTransaction(recipient, amount, 0)
				if err != nil {
					log.Error(err)
					return nil
				}
				//the nonce and amount have changed, let's save them
				senderWallet.ExportWallet(walletPath)
				log.Info("Generated Transaction")

				broadcast := &network.Broadcast{}
				broadcast.Type = network.Broadcast_TRANSACTION
				broadcast.Data = transaction

				bdata, err := proto.Marshal(broadcast)
				if err != nil {
					log.Error(err)
					return err
				}

				env := &network.Envelope{}
				env.Data = bdata
				env.Type = network.Envelope_BROADCAST

				data, err := proto.Marshal(env)
				if err != nil {
					log.Error(err)
					return err
				}

				log.Printf("Raw Transaction: %x", transaction)

				dial := websocket.Dialer{
					Proxy:            http.ProxyFromEnvironment,
					HandshakeTimeout: 5 * time.Second,
				}

				conn, _, err := dial.Dial("ws://localhost:8000/ws", nil)
				if err != nil {
					log.Error(err)
					return err
				}

				err = conn.WriteMessage(websocket.BinaryMessage, data)
				if err != nil {
					log.Error(err)
					return err
				}

				conn.Close()

				return nil
			},
		},
		{
			Name:    "makevanitywallet",
			Usage:   "mvw [wallet] [regex]",
			Aliases: []string{"mvw", "mv"},
			Action: func(c *cli.Context) error {
				log.Info("Dexm uses Base58 encoding, only chars allowed are 123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz")

				vanity := c.Args().Get(1)
				userWallet := c.Args().Get(0)

				if c.Args().Get(0) == "" {
					log.Fatal("Invalid filename")
				}

				vainityFound := false
				var wg sync.WaitGroup

				for i := 0; i < 4; i++ {
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
