package main

import (
	"fmt"
	"net/http"
	"os"
	"os/user"
	"strconv"
	"sync"
	"time"

	"github.com/dexm-coin/dexmd/networking"
	"github.com/gorilla/websocket"

	"github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/wallet"
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
					log.Fatal(err)
				}

				// Find the home folder of the current user
				user, err := user.Current()
				if err != nil {
					log.Fatal(err)
				}

				// Create the dexm folder in case it's not there
				os.MkdirAll(user.HomeDir+"/.dexm", os.ModePerm)

				// Create the blockchain database
				b, err := blockchain.NewBlockchain(user.HomeDir+"/.dexm", 0)
				if err != nil {
					log.Fatal(err)
				}

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
					log.Fatal(err)
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

				cs.UpdateChain()

				log.Info("Done importing")

				select {}

			},
		},

		{
			Name:    "maketransaction",
			Usage:   "mkt [walletPath] [recipient] [amount] [gas] [network]",
			Aliases: []string{"mkt", "gt"},
			Action: func(c *cli.Context) error {
				walletPath := c.Args().Get(0)
				recipient := c.Args().Get(1)
				amount, err := strconv.ParseUint(c.Args().Get(2), 10, 64)
				gas := strconv.ParseUint(c.Args().Get(3), 10, 64)
				networkName := c.Args().Get(4)

				if err != nil {
					log.Error(err)
					return nil
				}
				senderWallet, err := wallet.ImportWallet(walletPath)
				if err != nil {
					log.Error(err)
					return nil
				}

				ip, err := networking.GetPeerList(networkName)
				if err != nil {
					log.Error(err)
					return nil
				}

				dial := websocket.Dialer{
					Proxy:            http.ProxyFromEnvironment,
					HandshakeTimeout: 5 * time.Second,
				}
				conn, _, err := dial.Dial(fmt.Sprintf("ws://%s/ws", ip[0]), nil)
				if err != nil {
					return err
				}
				c := client{
					conn:      conn,
					send:      make(chan []byte, 256),
					readOther: make(chan []byte, 256),
					store:     nil,
				}

				env := &network.Envelope{
					Type: network.Request_GET_WALLET_STATUS,
					Data: []byte{},
				}

				accountState := c.send <- env
				senderWallet.Nonce = accountState.Nonce
				senderWallet.Balance = accountState.Balance

				transaction, err := senderWallet.NewTransaction(recipient, amount, gas)
				if err != nil {
					log.Error(err)
					return nil
				}
				//the nonce and amount have changed, let's save them
				senderWallet.ExportWallet(walletPath)

				env := &network.Envelope{
					Type: network.Envelope_REQUEST,
					Data: transaction,
				}

				c.send <- env
				
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
				cores, err := strconv.Atoi(c.Args().Get(2))

				if err != nil {
					log.Error(err)
					return nil
				}

				if c.Args().Get(0) == "" {
					log.Fatal("Invalid filename")
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
