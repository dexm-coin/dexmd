package blockchain

import (
	"bytes"
	"reflect"

	"github.com/dexm-coin/dexmd/wallet"
	bp "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/dexm-coin/wagon/exec"
	"github.com/dexm-coin/wagon/wasm"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
	"github.com/syndtr/goleveldb/leveldb"
)

var currentContract *Contract

// ReturnState tells the block generator what the contract outputed
type ReturnState struct {
	Reverted bool
	GasLeft  int
	Outputs  []*bp.Receipt
}

// Contract is the struct that saves the state of a contract
type Contract struct {
	ContractDb  *leveldb.DB
	StateDb     *leveldb.DB
	Code        []byte
	Address     []byte
	State       *bp.ContractState
	Block       *bp.Block
	Chain       *Blockchain
	Transaction *bp.Transaction
	Return      *ReturnState
	Sender      string

	TempDB      map[string][]byte
	TempBalance uint64

	Module *wasm.Module
	VM     *exec.VM
}

// GetContract loads the code and state from the DB and returns an error if there
// is no code. In case there is no state an empty one will be generated
func GetContract(address string, bc *Blockchain, tr *bp.Transaction, balance uint64) (*Contract, error) {
	code, err := bc.ContractDb.Get([]byte(address), nil)
	if err != nil {
		return nil, err
	}

	// Parse wasm code
	m, err := wasm.ReadModule(bytes.NewReader(code), setupImport)
	if err != nil {
		return nil, err
	}

	// Create empty context
	vm, err := exec.NewVM(m)
	if err != nil {
		return nil, err
	}

	// Don't panic, error instead
	vm.RecoverPanic = true

	state := bp.ContractState{}

	// Fetch from DB and use empty state if there is no state in the DB
	encodedState, err := bc.StateDb.Get([]byte(address), nil)
	if err != nil {
		state.Memory = vm.Memory()
		state.Globals = vm.Globals()
	} else {
		proto.Unmarshal(encodedState, &state)
	}

	senderAddr := wallet.BytesToAddress(tr.Sender, tr.Shard)

	return &Contract{
		Code:    code,
		Address: []byte(address),
		State:   &state,
		Return:  &ReturnState{},

		Module:      m,
		VM:          vm,
		Chain:       bc,
		Sender:      senderAddr,
		Transaction: tr,
		TempDB:      make(map[string][]byte),
		TempBalance: balance,
	}, nil
}

// ExecuteContract runs the function with the passed arguments
func (c *Contract) ExecuteContract(exportName string, arguments []uint64) *ReturnState {
	// Check if the contract is locked and the sender is authorized
	if c.Sender != string(c.State.AllowedWallet) && c.State.Locked {
		return c.Return
	}

	// Set the VM state before executing
	c.VM.SetMemory(c.State.Memory)
	c.VM.SetGlobal(c.State.Globals)
	// Set the current contract struct for the proc apis
	currentContract = c

	// Check if the passed function exists
	calledFunction, ok := c.Module.Export.Entries[exportName]
	if !ok {
		c.Return.Reverted = true
		return c.Return
	}

	log.Info(exportName, calledFunction.Index)

	// Call the function with passed arguments
	_, err := c.VM.ExecCode(int64(calledFunction.Index), arguments...)
	if err != nil {
		c.Return.Reverted = true
		return c.Return
	}

	// If the contract wasn't reverted save the new state
	if !c.Return.Reverted {
		c.State.Memory = c.VM.Memory()
		c.State.Globals = c.VM.Globals()
	} else {
		c.State.Locked = false
	}

	return nil
}

// SaveState saves the contract state to the database
// TODO Save tempdb
func (c *Contract) SaveState() error {
	state, _ := proto.Marshal(c.State)

	return c.StateDb.Put(c.Address, state, nil)
}

func setupImport(name string) (*wasm.Module, error) {
	m := wasm.NewModule()

	m.Types = &wasm.SectionTypes{
		Entries: []wasm.FunctionSig{
			// revert()
			{
				Form:        0,
				ParamTypes:  []wasm.ValueType{},
				ReturnTypes: []wasm.ValueType{},
			},

			// balance() : i64
			{
				Form:        1,
				ParamTypes:  []wasm.ValueType{},
				ReturnTypes: []wasm.ValueType{wasm.ValueTypeI64},
			},

			// pay(to_ptr: i32, amnt: i64, gas : i64)
			{
				Form: 2,
				ParamTypes: []wasm.ValueType{wasm.ValueTypeI32,
					wasm.ValueTypeI64, wasm.ValueTypeI64},
				ReturnTypes: []wasm.ValueType{},
			},

			// time() : i64
			{
				Form:        3,
				ParamTypes:  []wasm.ValueType{},
				ReturnTypes: []wasm.ValueType{wasm.ValueTypeI64},
			},

			// sender(ptr, size: i32)
			{
				Form:        4,
				ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32},
				ReturnTypes: []wasm.ValueType{},
			},

			// value() : i64
			{
				Form:        5,
				ParamTypes:  []wasm.ValueType{},
				ReturnTypes: []wasm.ValueType{wasm.ValueTypeI64},
			},

			// get(table, tlen, key, klen, dest, dlen: i32)
			{
				Form: 6,
				ParamTypes: []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32,
					wasm.ValueTypeI32, wasm.ValueTypeI32,
					wasm.ValueTypeI32, wasm.ValueTypeI32},
				ReturnTypes: []wasm.ValueType{},
			},
			// put(table, tlen, key, klen, data, dlen: i32)
			{
				Form: 7,
				ParamTypes: []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32,
					wasm.ValueTypeI32, wasm.ValueTypeI32,
					wasm.ValueTypeI32, wasm.ValueTypeI32},
				ReturnTypes: []wasm.ValueType{},
			},
			// data(to, sz: i32)
			{
				Form:        8,
				ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32},
				ReturnTypes: []wasm.ValueType{},
			},
		},
	}

	m.FunctionIndexSpace = []wasm.Function{
		// revert()
		{
			Sig:  &m.Types.Entries[0],
			Host: reflect.ValueOf(revert),
			Body: &wasm.FunctionBody{},
		},

		// balance() : i64
		{
			Sig:  &m.Types.Entries[1],
			Host: reflect.ValueOf(balance),
			Body: &wasm.FunctionBody{},
		},

		// pay(to_ptr: i32, amnt: i64)
		{
			Sig:  &m.Types.Entries[2],
			Host: reflect.ValueOf(pay),
			Body: &wasm.FunctionBody{},
		},

		// time() : i64
		{
			Sig:  &m.Types.Entries[3],
			Host: reflect.ValueOf(timestamp),
			Body: &wasm.FunctionBody{},
		},

		// sender(ptr: i64, size: i64) : i64
		{
			Sig:  &m.Types.Entries[4],
			Host: reflect.ValueOf(sender),
			Body: &wasm.FunctionBody{},
		},

		// value() : i64
		{
			Sig:  &m.Types.Entries[5],
			Host: reflect.ValueOf(value),
			Body: &wasm.FunctionBody{},
		},

		// get(table, tlen, key, klen, dest, dlen: i32)
		{
			Sig:  &m.Types.Entries[6],
			Host: reflect.ValueOf(get),
			Body: &wasm.FunctionBody{},
		},

		// put(table, tlen, key, klen, data, dlen: i32)
		{
			Sig:  &m.Types.Entries[7],
			Host: reflect.ValueOf(put),
			Body: &wasm.FunctionBody{},
		},

		// data(to, sz: i32)
		{
			Sig:  &m.Types.Entries[8],
			Host: reflect.ValueOf(data),
			Body: &wasm.FunctionBody{},
		},
	}

	m.Export = &wasm.SectionExports{
		Entries: map[string]wasm.ExportEntry{
			"revert": {
				FieldStr: "revert",
				Kind:     wasm.ExternalFunction,
				Index:    0,
			},

			"balance": {
				FieldStr: "balance",
				Kind:     wasm.ExternalFunction,
				Index:    1,
			},

			"pay": {
				FieldStr: "pay",
				Kind:     wasm.ExternalFunction,
				Index:    2,
			},

			"time": {
				FieldStr: "time",
				Kind:     wasm.ExternalFunction,
				Index:    3,
			},

			"sender": {
				FieldStr: "sender",
				Kind:     wasm.ExternalFunction,
				Index:    4,
			},
			"value": {
				FieldStr: "value",
				Kind:     wasm.ExternalFunction,
				Index:    5,
			},
			"get": {
				FieldStr: "get",
				Kind:     wasm.ExternalFunction,
				Index:    6,
			},
			"put": {
				FieldStr: "put",
				Kind:     wasm.ExternalFunction,
				Index:    7,
			},
			"data": {
				FieldStr: "data",
				Kind:     wasm.ExternalFunction,
				Index:    8,
			},
		},
	}

	return m, nil
}
