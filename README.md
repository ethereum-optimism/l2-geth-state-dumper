# geth-dumper

A special purpose geth state dumper which does a couple of strange things:

1. Deploys some initcode to a blank state
2. Creates a state dump
3. Replaces all instances of the initial contract addresses with some hardcoded addresses
4. Prints out the state dump

This is used for generating the initial OVM state (with an ExecutionManager & StateManager, etc) at specific addresses.

## Usage

### Generating State Dump Input (`deployment-tx-data.json`)
If you want to replace state dump input txs, then you'll need to generate a new `deployment-tx-data.json` file. Do this when you make changes to the ExecutionManager
and you'd like them reflected in the intial OVM state.

To do this, find the Geth Input Dump test file (https://github.com/ethereum-optimism/optimism-monorepo/blob/master/packages/contracts/test/deployment/geth-input-dump.spec.ts)
and change `.skip` to `.only` -- this will run the test & generate a `deployment-tx-data.json` file. That file contains all of the txs we want to apply to our initial state.

Next copy the `deployment-tx-data.json` into the root directory of this project.

### Generating a state dump
```
$ go install
$ go run main.go
.... # this prints out a lot of stuff...
653030633531303434663463623131326138623962346163376561623563313961227d7d7d7d

DUMP PRINTED! Copy sent to: state-dump.hex
JSON string version sent to: state-dump.json
To add to L2Geth, copy the dump hex into `ovm_constants.go`
```

### Ingesting the state dump
You can ingest either the JSON version of the state dump, or the hexified version of the state dump. Up to you!

## TODO
Make this process way cleaner & more automated. I am sorry for the poor tooling here!
