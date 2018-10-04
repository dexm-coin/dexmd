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

// timestamp returns the time the block was generated, could be manipulated by
// the block proposers within a range of 5s
func timestamp(proc *exec.Process) int64 {
	return int64(currentContract.Block.Timestamp)
}

// balance returns the balance of the contract, not including value()
func balance(proc *exec.Process) int64 {
	state, err := currentContract.Chain.GetWalletState(string(currentContract.Address))
	if err != nil {
		return 0
	}

	return int64(state.Balance)
}

// value returns the value of the transaction that called the current function
func value(proc *exec.Process) int64 {
	return int64(currentContract.Transaction.Amount)
}

// sender saves the caller of the current function to the specified pointer.
// if len(sender) > len then an error will be thrown. This is done to avoid
// memory corruption inside the contract.
func sender(proc *exec.Process, to, maxLen int32) int64 {
	senderAddr := wallet.BytesToAddress(currentContract.Transaction.Sender, currentContract.Transaction.Shard)

	if len(senderAddr) > int(maxLen) {
		revert(proc)
	}

	proc.WriteAt([]byte(senderAddr), int64(to))
	return 0
}

func pay(proc *exec.Process, to int32, amnt, gas int64) {
	reciver := readString(proc, to)
	log.Info("Transaction in contract to ", reciver, amnt, gas)
	return
}

func get(proc *exec.Process) {}

func set(proc *exec.Process) {}

func lock(proc *exec.Process, allowedAddr int32) {
	reciver := readString(proc, allowedAddr)

	// Not checking wallets could lead to contracts locked forever
	if !wallet.IsWalletValid(reciver) {
		revert(proc)
		return
	}

	currentContract.State.Locked = true
}

func unlock(proc *exec.Process) {
	currentContract.State.Locked = false
}

// Copies the data passed inside a transaction to the contract memory
func data(proc *exec.Process, to int32, sz int32) {
	if len(currentContract.Transaction.Data) > int(sz) {
		revert(proc)
	}
}

// Approves a binary patch to the contract by passing a BLAKE2b hash of the
// diff to this function. Afterwards it can be included inside a block and it
// will be accepted. Patching also locks the contract for a block.
func approvePatch(proc *exec.Process, hashPtr int32) {
	// This assumes a BLAKE-2b hash
	hash := make([]byte, 32)
	proc.ReadAt(hash, int64(hashPtr))
}

func readString(proc *exec.Process, ptr int32) string {
	// Read 256 bytes from memory
	maxLen := make([]byte, 256)
	proc.ReadAt(maxLen, int64(ptr))

	// Return string till the first \x00 byte
	return strings.TrimRight(string(maxLen), "\x00")
}
