package blockchain

import (
	"bytes"
	"errors"
	"reflect"

	bp "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/dexm-coin/wagon/exec"
	"github.com/dexm-coin/wagon/wasm"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
	"github.com/syndtr/goleveldb/leveldb"
)

var currentContract *Contract

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

	Module *wasm.Module
	VM     *exec.VM
}

// GetContract loads the code and state from the DB and returns an error if there
// is no code. In case there is no state an empty one will be generated
func GetContract(address string, contractDb, stateDb *leveldb.DB, bc *Blockchain, tr *bp.Transaction) (*Contract, error) {
	code, err := contractDb.Get([]byte(address), nil)
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

	state := bp.ContractState{}

	// Fetch from DB and use empty state if there is no state in the DB
	encodedState, err := stateDb.Get([]byte(address), nil)
	if err != nil {
		state.Memory = vm.Memory()
		state.Globals = vm.Globals()
	} else {
		proto.Unmarshal(encodedState, &state)
	}

	return &Contract{
		ContractDb: contractDb,
		StateDb:    stateDb,
		Code:       code,
		Address:    []byte(address),
		State:      &state,

		Module:      m,
		VM:          vm,
		Chain:       bc,
		Transaction: tr,
	}, nil
}

// ExecuteContract runs the function with the passed arguments
func (c *Contract) ExecuteContract(exportName string, arguments []uint64) error {
	// Set the VM state before executing
	c.VM.SetMemory(c.State.Memory)
	c.VM.SetGlobal(c.State.Globals)
	// Set the current contract struct for the proc apis
	currentContract = c

	// Check if the passed function exists
	calledFunction, ok := c.Module.Export.Entries[exportName]
	if !ok {
		return errors.New("Invalid export index")
	}

	log.Info(exportName, calledFunction.Index)

	// Call the function with passed arguments
	_, err := c.VM.ExecCode(int64(calledFunction.Index))
	if err != nil {
		return err
	}

	// Save the new state
	c.State.Memory = c.VM.Memory()
	c.State.Globals = c.VM.Globals()

	return nil
}

// SaveState saves the contract state to the database
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

			// pay(to_ptr: i64, amnt: i64, gas : i64)
			{
				Form: 2,
				ParamTypes: []wasm.ValueType{wasm.ValueTypeI64,
					wasm.ValueTypeI64, wasm.ValueTypeI64},
				ReturnTypes: []wasm.ValueType{},
			},

			// time() : i64
			{
				Form:        3,
				ParamTypes:  []wasm.ValueType{},
				ReturnTypes: []wasm.ValueType{wasm.ValueTypeI64},
			},

			// sender(ptr: i64, size: i64) : i64
			{
				Form:        4,
				ParamTypes:  []wasm.ValueType{wasm.ValueTypeI64, wasm.ValueTypeI64},
				ReturnTypes: []wasm.ValueType{wasm.ValueTypeI64},
			},

			// value() : i64
			{
				Form:        5,
				ParamTypes:  []wasm.ValueType{},
				ReturnTypes: []wasm.ValueType{wasm.ValueTypeI64},
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
		},
	}

	return m, nil
}
