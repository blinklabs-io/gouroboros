# cardano-node integration tests

These opt-in tests connect to a running `cardano-node` through its local
Node-to-Client UNIX socket. They verify the configured network handshake,
read a nonempty chain tip, decode a real block through ChainSync, and check
that ChainSync can be stopped and restarted on the connection.

The node must have a chain containing at least one block and must expose its
socket to the test process. The harness does not start or configure a node, so
it can be used with an existing testnet node or a custom testnet where blocks
are produced locally. Keep the socket path and network magic from the node's
own configuration; no public endpoint or network magic is assumed.

Run the suite from the repository root:

```sh
export GOUROBOROS_CARDANO_NODE_SOCKET_PATH=/path/to/node.socket
export GOUROBOROS_CARDANO_NETWORK_MAGIC=42
./internal/test/cardano-node-integration/run-tests.sh
```

The test also supports direct Go invocation:

```sh
GOUROBOROS_CARDANO_NODE_SOCKET_PATH=/path/to/node.socket \
GOUROBOROS_CARDANO_NETWORK_MAGIC=42 \
go test -tags=cardano_node_integration -v -count=1 \
  ./internal/test/cardano-node-integration
```

If either variable is missing, the tagged test skips when invoked directly.
The runner requires both values so a misconfigured integration run cannot pass
as a successful skip. The normal `go test ./...` and `make test` commands do
not include this suite because it requires an external node.
