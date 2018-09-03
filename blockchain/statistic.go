package blockchain

import (
	"fmt"
	"math/rand"
	"net/http"
	"os/user"
	"sort"
	"strconv"

	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
	"github.com/syndtr/goleveldb/leveldb"
)

type kv struct {
	Key   string
	Value int64
}

var globaldb *leveldb.DB
var currentBlock int64
var validators []kv
var totalStake int64

func readDb() (*leveldb.DB, error) {
	user, err := user.Current()
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	path := user.HomeDir + "/.dexm"

	db, err := leveldb.OpenFile(path+".balances", nil)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return db, nil
}

func getWalletsStatus() (string, []kv) {
	iter := globaldb.NewIterator(nil, nil)
	balances := map[string]int64{}

	for iter.Next() {
		key := iter.Key()
		value := iter.Value()

		state := protobufs.AccountState{}
		proto.Unmarshal(value, &state)

		wallet := fmt.Sprintf("%s", key)
		balances[wallet] = int64(state.Balance)
	}
	iter.Release()
	err := iter.Error()
	if err != nil {
		log.Error(err)
	}

	var ss []kv
	for k, v := range balances {
		ss = append(ss, kv{k, v})
	}
	sort.Slice(ss, func(i, j int) bool {
		return ss[i].Value > ss[j].Value
	})

	var walletstate string
	var tmpTotalStake = int64(0)
	for _, kv := range ss {
		walletstate += kv.Key + " -> " + strconv.Itoa(int(kv.Value)) + "\n"
		tmpTotalStake += kv.Value
	}
	totalStake = tmpTotalStake

	return walletstate, ss
}

func handlerWallets(w http.ResponseWriter, r *http.Request) {
	s, ss := getWalletsStatus()
	validators = ss
	fmt.Fprintf(w, s, r.URL.Path[1:])
}

func handlerValidator(w http.ResponseWriter, r *http.Request) {
	// winners := map[int64]string{}
	var winnersstring string
	// winnersstring += "Probability to be taken: " + strconv.Itoa(int(yourBalance * 100 / totalStake)) + "\n\n"
	for i := currentBlock; i <= currentBlock+50; i++ {
		rand.Seed(i)
		level := rand.Float64() * float64(totalStake)
		var counter int64
		for _, val := range validators {
			counter += val.Value
			if float64(counter) >= level {
				// winners[i] = val.Key
				winnersstring += "Round " + strconv.Itoa(int(i)) + " will win " + val.Key + "\n"
				break
			}
		}
	}

	// for k, v := range winners {
	// 	winnersstring += "Round " + strconv.Itoa(int(k)) + " will win " + v + "\n"
	// }

	fmt.Fprintf(w, winnersstring, r.URL.Path[1:])
}

// OpenService open a service in 3142 port where you can find all the balance and nonce of everyone in the network
func OpenService(CurrentBlock uint64) {
	db, err := readDb()
	if err != nil {
		log.Fatal(err)
	}
	globaldb = db
	currentBlock = int64(CurrentBlock)

	http.HandleFunc("/wallets", handlerWallets)
	http.HandleFunc("/validator", handlerValidator)
	http.ListenAndServe(":3142", nil)
}
