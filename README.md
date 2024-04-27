<div align="center">
  <img src="./assets/gOuroboros-logo-with-text-horizontal.png" alt="gOurobros Logo" width="640">
  <br>
  <img alt="GitHub" src="https://img.shields.io/github/license/blinklabs-io/gouroboros">
  <a href="https://pkg.go.dev/github.com/blinklabs-io/gouroboros"><img src="https://pkg.go.dev/badge/github.com/blinklabs-io/gouroboros.svg" alt="Go Reference"></a>
  <a href="https://discord.gg/5fPRZnX4qW"><img src="https://img.shields.io/badge/Discord-7289DA?style=flat&logo=discord&logoColor=white" alt="Discord"></a>
</div>

## Introduction

gOuroboros is a powerful and versatile framework for building Go apps that interact with the Cardano blockchain. Quickly and easily
write Go apps that communicate with Cardano nodes or manage blocks/transactions. Sync the blockchain from a local or remote node,
query a local node for protocol parameters or UTxOs by address, and much more.

## Features

This is not an exhaustive list of existing and planned features, but it covers the bulk of it.

- [ ] Ouroboros support
  - [ ] Muxer
    - [X] support for multiple mini-protocols over single connection
    - [X] support for separate initiator and responder instance for each protocol
    - [ ] support for buffer limits for each mini-protocol
  - [ ] Protocols
    - [X] Handshake
      - [X] Client support
      - [X] Server support
    - [X] Keepalive
      - [X] Client support
      - [X] Server support
    - [X] ChainSync
      - [X] Client support
      - [X] Server support
    - [X] BlockFetch
      - [X] Client support
      - [X] Server support
    - [X] TxSubmission
      - [X] Client support
      - [X] Server support
    - [X] PeerSharing
      - [X] Client support
      - [X] Server support
    - [X] LocalTxSubmission
      - [X] Client support
      - [X] Server support
    - [X] LocalTxMonitor
      - [X] Client support
      - [X] Server support
    - [ ] LocalStateQuery
      - [X] Client support
      - [ ] Server support
      - [ ] Queries
        - [X] System start
        - [X] Current era
        - [X] Chain tip
        - [X] Era history
        - [X] Current protocol parameters
        - [X] Stake distribution
        - [ ] Non-myopic member rewards
        - [ ] Proposed protocol parameter updates
        - [X] UTxOs by address
        - [ ] UTxO whole
        - [X] UTxO by TxIn
        - [ ] Debug epoch state
        - [ ] Filtered delegations and reward accounts
        - [X] Genesis config
        - [ ] Reward provenance
        - [X] Stake pools
        - [X] Stake pool params
        - [ ] Reward info pools
        - [ ] Pool state
        - [ ] Stake snapshots
        - [ ] Pool distribution
- [ ] Ledger
  - [ ] Eras
    - [ ] Byron
      - [X] Blocks
      - [X] Transactions
      - [X] TX inputs
      - [X] TX outputs
    - [X] Shelley
      - [X] Blocks
      - [X] Transactions
      - [X] TX inputs
      - [X] TX outputs
      - [X] Parameter updates
    - [X] Allegra
      - [X] Blocks
      - [X] Transactions
      - [X] TX inputs
      - [X] TX outputs
      - [X] Parameter updates
    - [X] Mary
      - [X] Blocks
      - [X] Transactions
      - [X] TX inputs
      - [X] TX outputs
      - [X] Parameter updates
    - [X] Alonzo
      - [X] Blocks
      - [X] Transactions
      - [X] TX inputs
      - [X] TX outputs
      - [X] Parameter updates
    - [X] Babbage
      - [X] Blocks
      - [X] Transactions
      - [X] TX inputs
      - [X] TX outputs
      - [X] Parameter updates
    - [X] Conway
      - [X] Blocks
      - [X] Transactions
      - [X] TX inputs
      - [X] TX outputs
      - [ ] Parameter updates
  - [ ] Transaction attributes
    - [X] Inputs
    - [X] Outputs
    - [X] Metadata
    - [X] Fees
    - [X] TTL
    - [ ] Certificates
    - [ ] Staking reward withdrawls
    - [X] Protocol parameter updates
    - [ ] Auxiliary data hash
    - [ ] Validity interval start
    - [ ] Mint operations
    - [ ] Script data hash
    - [ ] Collateral inputs
    - [ ] Required signers
    - [ ] Collateral return
    - [ ] Total collateral
    - [ ] Reference inputs
- [ ] Testing
  - [X] Test framework for mocking Ouroboros conversations
  - [ ] CBOR deserialization and serialization
    - [X] Protocol messages
    - [ ] Ledger
      - [ ] Block parsing
      - [ ] Transaction parsing
- [ ] Misc
  - [X] Address handling
    - [X] Decode from bech32
    - [X] Encode as bech32
    - [X] Deserialize from CBOR
    - [X] Retrieve staking key

## Testing

gOuroboros includes automated tests that cover various aspects of its functionality, but not all. For more than the basics,
manual testing is required.

### Running the automated tests

```
make test
```

### Manual testing

Various small test programs can be found in `cmd/` in this repo or in the [gOuroboros Starter Kit](https://github.com/blinklabs-io/gouroboros-starter-kit) repo.
Some of them can be run against public nodes via NtN protocols, but some may require access to the UNIX socket of a local node for NtC protocols.

#### Run chain-sync from the start of a particular era

This is useful for testing changes to the handling of ledger types for a particular era. It will decode each block and either print
a summary line for the block or an error.

Start by building the test programs:

```
make
```

Run the chain-sync test program against a public mainnet node, starting at the beginning of the Shelley era:

```
./gouroboros -address backbone.cardano-mainnet.iohk.io:3001 -network mainnet -ntn chain-sync -bulk -start-era shelley
```

This will produce a LOT of output and take quite a few hours to reach chain tip. You're mostly looking for it to get through
all blocks of the chosen start era before hitting the next era or chain tip

#### Dump details of a particular block

You can use the `block-fetch` program from `gouroboros-starter-kit` to fetch a particular block and dump its details. You must provide at least
the block slot and hash. This is useful for debugging decoding problems, since it allows fetching a specific block and decoding it over and over.

```
BLOCK_FETCH_SLOT=120521627 BLOCK_FETCH_HASH=afd4c97e32003d9803a305011cbd8796e6b36bf61576567206887e35795b6e09 ./block-fetch
```
