# geth-dumper

A special purpose geth state dumper which does a couple of strange things:

1. Deploys some initcode to a blank state
2. Creates a state dump
3. Replaces all instances of the initial contract addresses with some hardcoded addresses
4. Prints out the state dump

This is used for generating the initial OVM state (with an ExecutionManager & StateManager, etc) at specific addresses.
