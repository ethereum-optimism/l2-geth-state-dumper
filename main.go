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

type AddressUpdateMap struct {
	oldAddressToNewAddress map[common.Address]common.Address
	newAddressToOldAddress map[common.Address]common.Address
}

func (a AddressUpdateMap) getNewAddressIfExists(oldAddress common.Address) (common.Address, bool) {
	newAddr, found := a.oldAddressToNewAddress[oldAddress]
	return newAddr, found
}

func (a AddressUpdateMap) getNewAddress(oldAddress common.Address) common.Address {
	newAddr, _ := a.oldAddressToNewAddress[oldAddress]
	return newAddr
}

func (a AddressUpdateMap) associate(oldAddress common.Address, newAddress common.Address) {
	fmt.Println("Mapping:", hex.EncodeToString(oldAddress.Bytes()), "to", hex.EncodeToString(newAddress.Bytes()))
	a.oldAddressToNewAddress[oldAddress] = newAddress
	a.newAddressToOldAddress[newAddress] = oldAddress
}

func (a AddressUpdateMap) associateExisting(oldAddress common.Address, newAddress common.Address) {
	fmt.Println("Associating Existing:", hex.EncodeToString(oldAddress.Bytes()), "to", hex.EncodeToString(newAddress.Bytes()))
	displacedOldAddress := a.newAddressToOldAddress[newAddress]
	displacedNewAddress := a.oldAddressToNewAddress[oldAddress]
	a.associate(displacedOldAddress, displacedNewAddress)
	a.associate(oldAddress, newAddress)
}

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
var expectedExecutionMgrAddress = common.HexToAddress("a193e42526f1fea8c99af609dceabf30c1c29faa")
var desiredExecutionMgrAddress = common.HexToAddress("00000000000000000000000000000000dead0000")
var expectedStateMgrAddress = common.HexToAddress("0ddd780a2899b9a6b7acfe5153675cf65c55e03d")
var desiredStateMgrAddress = common.HexToAddress("00000000000000000000000000000000dead0001")
var startingDeadAddress = common.HexToAddress("00000000000000000000000000000000dead0000")

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

	// Apply to the state
	applyMessageToState(currentState, defaultDeployerAddress, zeroAddress, gasLimit, genesisInitcode)

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
	addressUpdateMap := AddressUpdateMap{
		newAddressToOldAddress: make(map[common.Address]common.Address),
		oldAddressToNewAddress: make(map[common.Address]common.Address),
	}

	idx := int64(0)
	for oldAddr := range theDump.Accounts {
		indexAsBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(indexAsBytes, uint64(idx))
		newAddr := common.BytesToAddress(startingDeadAddress.Bytes())
		// Replace the bytes in our `newAddr`
		for i, theByte := range indexAsBytes[:2] {
			newAddr[len(newAddr)-i-1] = theByte
		}

		addressUpdateMap.associate(oldAddr, newAddr)

		idx++
	}

	// Re-associate the ExecutionMgr and StateMgr addresses to always be dead0000 & dead0001
	addressUpdateMap.associateExisting(expectedExecutionMgrAddress, desiredExecutionMgrAddress)
	addressUpdateMap.associateExisting(expectedStateMgrAddress, desiredStateMgrAddress)

	// Next populate an updated dump with all the information we want
	updatedDump.Accounts = map[common.Address]state.DumpAccount{}
	// Modify the dump to replace addresses with our new addresses
	for addr, account := range theDump.Accounts {
		updatedDump.Accounts[addressUpdateMap.getNewAddress(addr)] = account
		for key, value := range account.Storage {
			fmt.Println("Addr", hex.EncodeToString(addr.Bytes()), "Key: ", hex.EncodeToString(key.Bytes()), "Value", value)
			if newAddress, found := addressUpdateMap.getNewAddressIfExists(common.HexToAddress(value)); found {
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
	fmt.Println("Return val: [HIDDEN]", "Gas used:", gasUsed, "Failed:", failed, "Error:", err)

	commitHash, commitErr := currentState.Commit(false)
	fmt.Println("Commit hash:", commitHash, "Commit err:", commitErr)

	return returnValue, gasUsed, failed, err
}
