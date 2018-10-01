package blockchain

import (
	"strings"

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
	return 1447
}

func value(proc *exec.Process) int64 {
	return 1447
}

func sender(proc *exec.Process, to, len int64) int64 {
	proc.WriteAt([]byte("AYYY\x00"), to)
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
