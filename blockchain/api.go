package blockchain

import (
	"strings"

	"github.com/dexm-coin/dexmd/wallet"
	"github.com/dexm-coin/wagon/exec"
	log "github.com/sirupsen/logrus"
)

func revert(proc *exec.Process) {
	proc.Terminate()
}

func timestamp(proc *exec.Process) int64 {
	return int64(currentContract.Block.Timestamp)
}

func balance(proc *exec.Process) int64 {
	state, err := currentContract.Chain.GetWalletState(string(currentContract.Address))
	if err != nil {
		return 0
	}

	return int64(state.Balance)
}

func value(proc *exec.Process) int64 {
	return int64(currentContract.Transaction.Amount)
}

func sender(proc *exec.Process, to, len int64) int64 {
	senderAddr := wallet.BytesToAddress(currentContract.Transaction.Sender, currentContract.Transaction.Shard)
	proc.WriteAt([]byte(senderAddr), to)
	return 0
}

func readString(proc *exec.Process, ptr int64) string {
	// Read 256 bytes from memory
	maxLen := make([]byte, 256)
	proc.ReadAt(maxLen, ptr)

	// Return string till the first \x00 byte
	return strings.TrimRight(string(maxLen), "\x00")
}

func pay(proc *exec.Process, to, amnt, gas int64) {
	reciver := readString(proc, to)
	log.Info("Transaction in contract to ", reciver, amnt, gas)
	return
}
