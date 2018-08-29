package contracts

import (
	"strings"

	"github.com/dexm-coin/wagon/exec"
	log "github.com/sirupsen/logrus"
)

func pay(proc *exec.Process, to, amnt, gas int) {
	reciver := readString(proc, to)
	log.Info("Transaction in contract to ", reciver, amnt, gas)
	return
}

func revert(proc *exec.Process) {
	proc.Terminate()
}

func time(proc *exec.Process) int {
	return 1337
}

func balance(proc *exec.Process) int {
	return 1447
}

func value(proc *exec.Process) int {
	return 1447
}

func sender(proc *exec.Process, to, len int) int {
	return 0
}

func readString(proc *exec.Process, ptr int) string {
	// Read 256 bytes from memory
	maxLen := make([]byte, 256)
	proc.ReadAt(maxLen, int64(ptr))

	// Return string till the first \x00 byte
	return strings.TrimRight(string(maxLen), "\x00")
}
