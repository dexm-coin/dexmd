package main

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abiosoft/ishell"
	"github.com/dexm-coin/dexmd/networking"

	"github.com/dexm-coin/dexmd/blockchain"
	"github.com/dexm-coin/dexmd/contracts"
	"github.com/dexm-coin/dexmd/wallet"
	bp "github.com/dexm-coin/protobufs/build/blockchain"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const (
	PORT              = 3141
	PUBLIC_PEERSERVER = false
)

var (
	TS = uint64(1537809073)
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
			Usage:   "sn [wallet] [timestamp] [network]",
			Aliases: []string{"sn", "rn"},
			Action: func(c *cli.Context) error {
				walletPath := c.Args().Get(0)
				genesisTimestamp := c.Args().Get(1)
				network := c.Args().Get(2)

				if network == "" {
					network = "hackney"
				}

				// Import an identity to encrypt data and sign for validator msg
				w, err := wallet.ImportWallet(walletPath)
				if err != nil {
					log.Fatal("import", err)
				}

				if genesisTimestamp != "" {
					TS, err = strconv.ParseUint(genesisTimestamp, 10, 64)
					if err != nil {
						log.Fatal(err)
					}
				}

				// Find the home folder of the current user
				// user, err := user.Current()
				// if err != nil {
				// 	log.Fatal("user", err)
				// }

				// Create the dexm folder in case it's not there
				os.MkdirAll(".dexm.shard", os.ModePerm)
				os.MkdirAll(".dexm.beacon", os.ModePerm)

				// Create the blockchain database
				b, err := blockchain.NewBlockchain(".dexm.shard/", 0)
				if err != nil {
					log.Fatal("blockchain", err)
				}

				// Create the blockchain database
				beacon, err := blockchain.NewBeaconChain(".dexm.beacon/")
				if err != nil {
					log.Fatal("blockchain", err)
				}

				log.Info("Adding genesis block...")

				log.Info(time.Now().Unix())

				genesisBlock := &bp.Block{
					Index:     0,
					Timestamp: TS,
					Miner:     "Dexm3ENiLVMNwaeRswEbV1PT7UEpDNwwlbef2e683",
				}
				b.SaveBlock(genesisBlock)

				// Open the port on the router, ignore errors
				networking.TraverseNat(PORT, "Dexm Blockchain Node")

				// Open a client on the default port
				cs, err := networking.StartServer(
					fmt.Sprintf(":%d", PORT),
					network,
					b,
					beacon,
					w,
				)
				if err != nil {
					log.Fatal("start", err)
				}

				cs.ImportBlock(genesisBlock)

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

				ccreation := len(cdata) == 0

				networking.SendTransaction(senderWallet, recipient, "", amount, uint64(gas), cdata, ccreation)

				return nil
			},
		},

		{
			Name:    "interact",
			Usage:   "i [address]",
			Aliases: []string{"i"},
			Action: func(c *cli.Context) error {
				walPath := c.Args().Get(0)
				address := c.Args().Get(1)

				senderWallet, err := wallet.ImportWallet(walPath)
				if err != nil {
					log.Fatal(err)
				}

				// Find the home folder of the current user
				// user, err := user.Current()
				// if err != nil {
				// 	log.Fatal(err)
				// }

				b, err := blockchain.NewBlockchain(".dexm.shard/", 0)
				if err != nil {
					log.Fatal("nb", err)
					return nil
				}

				contract, err := contracts.GetContract(address, b.ContractDb, b.StateDb)
				if err != nil {
					log.Fatal(err)
					return nil
				}

				log.Info("Inspecting ", address)

				shell := ishell.New()

				var entries []string
				for key := range contract.Module.Export.Entries {
					entries = append(entries, key)
				}

				var choice int
				shell.AddCmd(&ishell.Cmd{
					Name: "entries",
					Help: "Function entries from the contract",
					Func: func(c *ishell.Context) {
						choice = c.MultiChoice(entries, "Which function do you want to use ?")

						c.Println("Insert the transaction value")
						valS := c.ReadLine()

						c.Println("Insert gas cost")
						gasS := c.ReadLine()

						amount, err := strconv.ParseUint(valS, 10, 64)
						if err != nil {
							log.Fatal(err)
						}

						gas, err := strconv.Atoi(gasS)
						if err != nil {
							log.Fatal(err)
						}

						networking.SendTransaction(senderWallet, address, entries[choice], amount, uint64(gas), []byte{}, false)
					},
				})

				shell.AddCmd(&ishell.Cmd{
					Name: "memory",
					Help: "Inspect the contract memory",
					Func: func(c *ishell.Context) {
						log.Print(hex.Dump(contract.State.GetMemory()))
					},
				})

				shell.AddCmd(&ishell.Cmd{
					Name: "globals",
					Help: "Inspect the contract globals",
					Func: func(c *ishell.Context) {
						for k, v := range contract.State.GetGlobals() {
							log.Println(k, "->", v)
						}
					},
				})

				shell.Run()

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

		/* {
			Name:    "withdraw",
			Usage:   "wd [walletPath]",
			Aliases: []string{"withdraw", "wd"},
			Action: func(c *cli.Context) error {
				walletPath := c.Args().Get(0)
				w, err := wallet.ImportWallet(walletPath)
				if err != nil {
					log.Error("import", err)
					return nil
				}


				return nil
			},
		}, */
	}

	app.Run(os.Args)
}
