package main

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
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
	// Pull off a deployer address from the args if they care
	deployerAddress := defaultDeployerAddress
	if len(os.Args) >= 2 {
		deployerAddress = common.HexToAddress(os.Args[1])
	}
	applyMessageToState(currentState, deployerAddress, zeroAddress, gasLimit, genesisInitcode)

	// Create the dump
	theDump := currentState.RawDump(false, false, false)
	fmt.Println("Dump root:", theDump.Root)

	// Convert the dump to change all addresses to be DEAD
	updatedDump := replaceDumpAddresses(theDump)

	fmt.Println("\nDUMP INCOMING!")
	marshaledDump, _ := json.Marshal(updatedDump)
	fmt.Println(common.Bytes2Hex(marshaledDump))
}

func replaceDumpAddresses(theDump state.Dump) (updatedDump state.Dump) {
	// First generate all of the replacement addresses
	newAddresses := map[common.Address]common.Address{}
	startingAddress := common.HexToAddress("00000000000000000000000000000000dead0000")
	idx := int64(0)
	for addr := range theDump.Accounts {
		indexAsBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(indexAsBytes, uint64(idx))
		fmt.Println(indexAsBytes)

		newAddr := startingAddress.Bytes()

		for i, theByte := range indexAsBytes[:2] {
			// for i := 1; i <= len(indexAsBytes); i++ {
			newAddr[len(newAddr)-i-1] = theByte
			fmt.Println(newAddr[len(newAddr)-i-1])
			// newAddr[len(newAddr)-i] = indexAsBytes[len(indexAsBytes)-i]
		}
		// Map the old addresses to the new!
		newAddresses[addr] = common.BytesToAddress(newAddr)
    fmt.Println("Mapped:", hex.EncodeToString(addr.Bytes()), "to", hex.EncodeToString(newAddresses[addr].Bytes()))
		idx++
	}

	// Next populate an updated dump with all the information we want
	updatedDump.Accounts = map[common.Address]state.DumpAccount{}
	// Modify the dump to replace addresses with our new addresses
	for addr, account := range theDump.Accounts {
		updatedDump.Accounts[newAddresses[addr]] = account
		for key, value := range account.Storage {
			fmt.Println("Addr", hex.EncodeToString(addr.Bytes()), "Key: ", hex.EncodeToString(key.Bytes()), "Value", value)
			if newAddress, found := newAddresses[common.HexToAddress(value)]; found {
				fmt.Println("Replacing", value, "with", newAddress)
				account.Storage[key] = newAddress.String()
			}
		}
	}
	return updatedDump
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
