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
	"github.com/ethereum/go-ethereum/common/hexutil"
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

type SimplifiedTx struct {
	From string
	To   string
	Data string
}

type GethDumpInput struct {
	SimplifiedTxs           []SimplifiedTx
	WalletAddress           string
	ExecutionManagerAddress string
	StateManagerAddress     string
	CodeHashes              map[string]string
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
	if _, ok := a.oldAddressToNewAddress[oldAddress]; !ok {
		fmt.Println("ERROR! Old address not found")
		os.Exit(1)
	}
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
var expectedExecutionMgrAddress common.Address
var desiredExecutionMgrAddress = common.HexToAddress("00000000000000000000000000000000dead0000")
var expectedStateMgrAddress common.Address
var desiredStateMgrAddress = common.HexToAddress("00000000000000000000000000000000dead0001")
var startingDeadAddress = common.HexToAddress("00000000000000000000000000000000dead0000")

var l2ToL1MessagePasser = common.HexToAddress("4200000000000000000000000000000000000000")
var l1MessageSender = common.HexToAddress("4200000000000000000000000000000000000001")

var l1CodeHash, l2CodeHash string

const gasLimit = 15000000

func readingGenesisError(err error) {
	fmt.Fprintf(os.Stderr, "Error reading genesis initcode: %v\n", err)
	os.Exit(1)
}

func main() {
	db := state.NewDatabase(rawdb.NewMemoryDatabase())
	currentState, _ := state.New(common.Hash{}, db)

	// Now get our initcode!
	dataStr, err := ioutil.ReadFile("deployment-tx-data.json")
	if err != nil {
		readingGenesisError(err)
	}

	var gethDumpInput GethDumpInput

	err = json.Unmarshal(dataStr, &gethDumpInput)

	deployerAddress := common.HexToAddress(gethDumpInput.WalletAddress)
	expectedExecutionMgrAddress = common.HexToAddress(gethDumpInput.ExecutionManagerAddress)
	expectedStateMgrAddress = common.HexToAddress(gethDumpInput.StateManagerAddress)

	l2CodeHash, _ = gethDumpInput.CodeHashes["l2ToL1MessagePasser"]
	l1CodeHash, _ = gethDumpInput.CodeHashes["l1MessageSender"]

	// Apply all the transactions to the state
	for _, simpleTx := range gethDumpInput.SimplifiedTxs {
		txData, err := hexutil.Decode(simpleTx.Data)
		if err != nil {
			readingGenesisError(err)
		}
		sender := common.HexToAddress(simpleTx.To)

		// Apply to the state
		applyMessageToState(currentState, deployerAddress, sender, gasLimit, txData)
	}

	// Create the dump
	theDump := currentState.RawDump(false, false, false)
	fmt.Println("Dump root:", theDump.Root)

	// Convert the dump to change all addresses to be DEAD
	updatedDump = replaceDumpAddresses(theDump)

	l2GethStateDumpFilename := "state-dump.hex"
	jsonStateDumpFilename := "state-dump.json"
	marshaledDump, _ := json.Marshal(updatedDump)
	fmt.Println("\nDUMP INCOMING!")
	fmt.Println(common.Bytes2Hex(marshaledDump))
	fmt.Println("\nDUMP PRINTED! Copy sent to:", l2GethStateDumpFilename)
	fmt.Println("JSON string version sent to:", jsonStateDumpFilename)
	fmt.Println("To add to L2Geth, copy the dump hex into `ovm_constants.go`")
	dumpOutput := []byte(common.Bytes2Hex(marshaledDump))
	ioutil.WriteFile(l2GethStateDumpFilename, dumpOutput, 0644)
	ioutil.WriteFile(jsonStateDumpFilename, []byte(string(marshaledDump)), 0644)
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

	for address, account := range theDump.Accounts {
		switch "0x" + account.CodeHash {
		case l1CodeHash:
			addressUpdateMap.associateExisting(address, l1MessageSender)
		case l2CodeHash:
			addressUpdateMap.associateExisting(address, l2ToL1MessagePasser)
		}
	}

	// Next populate an updated dump with all the information we want
	updatedDump.Accounts = map[common.Address]state.DumpAccount{}
	// Modify the dump to replace addresses with our new addresses
	for addr, account := range theDump.Accounts {
		updatedDump.Accounts[addressUpdateMap.getNewAddress(addr)] = account
		for key, value := range account.Storage {
			fmt.Println("Addr", hex.EncodeToString(addr.Bytes()), "Key: ", hex.EncodeToString(key.Bytes()), "Value", value)
			if newAddress, found := addressUpdateMap.getNewAddressIfExists(common.HexToAddress(value)); found {
				fmt.Println("Replacing", value, "with", hex.EncodeToString(newAddress.Bytes()))
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
