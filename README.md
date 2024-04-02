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
    - [X] Allegra
      - [X] Blocks
      - [X] Transactions
      - [X] TX inputs
      - [X] TX outputs
    - [X] Mary
      - [X] Blocks
      - [X] Transactions
      - [X] TX inputs
      - [X] TX outputs
    - [X] Alonzo
      - [X] Blocks
      - [X] Transactions
      - [X] TX inputs
      - [X] TX outputs
    - [X] Babbage
      - [X] Blocks
      - [X] Transactions
      - [X] TX inputs
      - [X] TX outputs
    - [X] Conway
      - [X] Blocks
      - [X] Transactions
      - [X] TX inputs
      - [X] TX outputs
  - [ ] Transaction attributes
    - [X] Inputs
    - [X] Outputs
    - [X] Metadata
    - [X] Fees
    - [X] TTL
    - [ ] Certificates
    - [ ] Staking reward withdrawls
    - [ ] Protocol parameter updates
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

Testing is currently a mostly manual process. There's an included test program that use the library
and a Docker Compose file to launch a local `cardano-node` instance.

### Starting the local `cardano-node` instance

```
$ docker-compose up -d
```

If you want to use `mainnet`, set the `CARDANO_NETWORK` environment variable.

```
$ export CARDANO_NETWORK=mainnet
$ docker-compose up -d
```

You can communicate with the `cardano-node` instance on port `8081` (for "public" node-to-node protocol), port `8082` (for "private" node-to-client protocol), or
the `./tmp/cardano-node/ipc/node.socket` UNIX socket file (also for "private" node-to-client protocol).

NOTE: if using the UNIX socket file, you may need to adjust the permissions/ownership to allow your user to access it.
The `cardano-node` Docker image runs as `root` by default and the UNIX socket ends up with `root:root` ownership
and `0755` permissions, which doesn't allow a non-root use to write to it by default.

### Running `cardano-cli` against local `cardano-node` instance

```
$ docker exec -ti gouroboros-cardano-node-1 sh -c 'CARDANO_NODE_SOCKET_PATH=/ipc/node.socket cardano-cli query tip --testnet-magic 1097911063'
```

### Building and running the test program

Compile the test program.

```
$ make
```

Run the test program pointing to the UNIX socket (via `socat`) from the `cardano-node` instance started above.

```
$ ./gouroboros -address localhost:8082 -network testnet ...
```

Run it against the public port in node-to-node mode.

```
$ ./gouroboros -address localhost:8081 -ntn -network testnet ...
```

Test chain-sync (works in node-to-node and node-to-client modes).

```
$ ./gouroboros ... chain-sync -start-era byron
```

Test local-tx-submission (only works in node-to-client mode).

```
$ ./gouroboros ... local-tx-submission ...
```

Test following the chain tip in the `preview` network.

```
$ ./gouroboros -network preview -address preview-node.world.dev.cardano.org:30002 -ntn chain-sync -tip
```

### Stopping the local `cardano-node` instance

```
$ docker-compose down --volumes
```
