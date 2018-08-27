package main

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/dexm-coin/dexmd/networking"

	"github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/wallet"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const (
	PORT = 3141
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

				// Create the user folder
				os.Mkdir("~/.dexm", os.ModePerm)

				// Create the blockchain database
				b, err := blockchain.NewBlockchain("~/.dexm/chaindata", 0)
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

				cs.FindPeers()
				cs.UpdateChain()

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
				_ = transaction
				//the nonce and amount have changed, let's save them
				senderWallet.ExportWallet(walletPath)
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
