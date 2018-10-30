package blockchain

import (
	"crypto/sha256"
	"strings"

	"github.com/dexm-coin/dexmd/util"
	"github.com/dexm-coin/dexmd/wallet"
	bp "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/dexm-coin/wagon/exec"
)

func revert(proc *exec.Process) {
	proc.Terminate()
	currentContract.Return.Reverted = true
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

	if !currentContract.IsTempBalanceSet {
		currentContract.TempBalance = state.Balance
		currentContract.IsTempBalanceSet = true
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

// Executes the transaction
func pay(proc *exec.Process, to int32, amnt, gas int64) {
	reciver := readString(proc, to)

	// Don't allow sending money to invalid wallets.
	if !wallet.IsWalletValid(reciver) {
		revert(proc)
	}

	// If the temp balance is not set then save it inside the struct
	if !currentContract.IsTempBalanceSet {
		balance(proc)
	}

	// Avoid overflows while summing balance
	requiredBal, overflow := util.AddI64O(amnt, gas)
	if overflow {
		revert(proc)
		return
	}

	// Check if balance is sufficient
	if uint64(requiredBal) >= currentContract.TempBalance {
		revert(proc)
		return
	}

	receipt := &bp.Receipt{}
	currentContract.Return.Outputs = append(currentContract.Return.Outputs, receipt)

	return
}

// Queries data from the KV Database within the contract, this is public
func get(proc *exec.Process, tableName, tableLen, key, keyLen, dest, destLen int32) int32 {
	// Limit destination size
	if destLen > 2048 {
		revert(proc)
		return 0
	}

	k := getKey(proc, tableName, tableLen, key, keyLen)

	// Handle getting and setting
	val, ok := currentContract.TempDB[string(k)]

	if !ok {
		dbVal, err := currentContract.StateDb.Get(k, nil)
		if err != nil {
			return 0
		}

		val = dbVal
	}

	// Avoid memory corruption by stopping execution before copying
	if len(val) > int(destLen) {
		revert(proc)
		return 0
	}

	proc.WriteAt(val, int64(dest))
	return 1
}

// Saves data in the K/V Database
func put(proc *exec.Process, table, tableLen, key, keyLen, data, dataLen int32) {
	if dataLen > 2048 {
		revert(proc)
		return
	}

	value := make([]byte, dataLen)
	proc.ReadAt(value, int64(data))

	// Save the data in a temporary map and save it only once the execution is over
	// this way if the contract reverts there is no state corruption
	stateKey := getKey(proc, table, tableLen, key, keyLen)
	currentContract.TempDB[string(stateKey)] = value
}

// Reads a table and a key and hashes them together with the address, generating
// an unique identifier for the entire blockchain
func getKey(proc *exec.Process, table, tableLen, key, keyLen int32) []byte {
	if keyLen > 2048 || tableLen > 2048 {
		revert(proc)
	}

	keyBuf := make([]byte, keyLen)
	proc.ReadAt(keyBuf, int64(key))

	tableBuf := make([]byte, tableLen)
	proc.ReadAt(tableBuf, int64(table))

	stateKey := append(keyBuf, currentContract.Address...)
	h := sha256.New()
	return h.Sum(stateKey)
}

// Only let a single approved address to call the contract. This is intended
// to be used in case a security vulnerability is discovered.
func lock(proc *exec.Process, allowedAddr int32) {
	reciver := readString(proc, allowedAddr)

	// Not checking wallets could lead to contracts locked forever
	if !wallet.IsWalletValid(reciver) {
		revert(proc)
		return
	}

	currentContract.State.AllowedWallet = []byte(reciver)
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

	proc.WriteAt(currentContract.Transaction.Data, int64(to))
}

func sha(proc *exec.Process, data, sz, dest int32) {}

// Approves a binary patch to the contract by passing a BLAKE2b hash of the
// diff to this function. Afterwards it can be included inside a block and it
// will be accepted. Patching also locks the contract for a block. TODO
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
