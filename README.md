# gouroboros

A Go client implementation of the Cardano Ouroboros network protocol

This is loosely based on the [official Haskell implementation](https://github.com/input-output-hk/ouroboros-network)

NOTE: this library is under heavily development, and the interface should not be considered stable until it reaches `v1.0.0`

## Implementation status

The Ouroboros protocol consists of a simple multiplexer protocol and various mini-protocols that run on top of it.
This makes it easy to implement only parts of the protocol without negatively affecting usability of this library.

The multiplexer and handshake mini-protocol are "fully" working. The focus will be on the node-to-client (local) protocols,
but the node-to-node protocols will also be implemented in time.

### Mini-protocols

| Name | Status |
| --- | --- |
| Handshake | Implemented |
| ChainSync | Implemented |
| BlockFetch | Implemented |
| TxSubmission2 | Not Implemented |
| LocalTxSubmission | Implemented |
| LocalStateQuery | Partly Implemented |
| KeepAlive | Implemented |
| LocalTxMonitor | Implemented |

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
