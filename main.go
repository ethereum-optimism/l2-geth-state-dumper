package main

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
)

var chainConfig params.ChainConfig

func init() {
	chainConfig = params.ChainConfig{
		ChainID:             big.NewInt(1),
		HomesteadBlock:      new(big.Int),
		ByzantiumBlock:      new(big.Int),
		ConstantinopleBlock: new(big.Int),
		DAOForkBlock:        new(big.Int),
		DAOForkSupport:      false,
		EIP150Block:         new(big.Int),
		EIP155Block:         new(big.Int),
		EIP158Block:         new(big.Int),
	}
}

var zeroAddress = common.HexToAddress("0000000000000000000000000000000000000000")
var defaultDeployerAddress = common.HexToAddress("17ec8597ff92C3F44523bDc65BF0f1bE632917ff")

const gasLimit = 15000000

func readingGenesisError(err error) {
	fmt.Fprintf(os.Stderr, "Error reading genesis initcode: %v\n", err)
	os.Exit(1)
}

func main() {
	db := state.NewDatabase(rawdb.NewMemoryDatabase())
	currentState, _ := state.New(common.Hash{}, db)

	// Now get our initcode!
	dataStr, err := ioutil.ReadFile("genesisInitcode.hex")
	if err != nil {
		readingGenesisError(err)
	}
	// Strip newline
	dataStr = dataStr[:len(dataStr)-1]
	genesisInitcode, err := hex.DecodeString(string(dataStr))
	if err != nil {
		readingGenesisError(err)
	}
	applyMessageToState(currentState, defaultDeployerAddress, zeroAddress, gasLimit, genesisInitcode)

	// Now print everything out
	fmt.Println("\nDUMP INCOMING!")
	theDump := currentState.RawDump(false, false, false)
	fmt.Println(theDump.Root)
	for addr := range theDump.Accounts {
		fmt.Println("Address:", hex.EncodeToString(addr.Bytes()))
	}
}

func applyMessageToState(currentState *state.StateDB, from common.Address, to common.Address, gasLimit uint64, data []byte) ([]byte, uint64, bool, error) {
	header := &types.Header{
		Number:     big.NewInt(0),
		Difficulty: big.NewInt(0),
	}
	gasPool := core.GasPool(100000000)
	// Generate the message
	message := types.Message{}
	if to == zeroAddress {
		// Check if to the zeroAddress, if so, make it nil
		message = types.NewMessage(
			from,
			nil,
			currentState.GetNonce(from),
			big.NewInt(0),
			gasLimit,
			big.NewInt(0),
			data,
			false,
		)
	} else {
		// Otherwise we actually use the `to` field!
		message = types.NewMessage(
			from,
			&to,
			currentState.GetNonce(from),
			big.NewInt(0),
			gasLimit,
			big.NewInt(0),
			data,
			false,
		)
	}

	context := core.NewEVMContext(message, header, nil, &from)
	evm := vm.NewEVM(context, currentState, &chainConfig, vm.Config{})

	returnValue, gasUsed, failed, err := core.ApplyMessage(evm, message, &gasPool)
	// fmt.Println("Return val:", returnValue, "Gas used:", gasUsed, "Failed:", failed, "Error:", err)
	fmt.Println("Return val: [HIDDEN]", "Gas used:", gasUsed, "Failed:", failed, "Error:", err)

	commitHash, commitErr := currentState.Commit(false)
	fmt.Println("Commit hash:", commitHash, "Commit err:", commitErr)

	return returnValue, gasUsed, failed, err
}
