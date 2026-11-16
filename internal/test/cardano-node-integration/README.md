# cardano-node integration tests

These opt-in tests connect to a running `cardano-node` through its local
Node-to-Client UNIX socket. They verify the configured network handshake,
read a nonempty chain tip, decode a real block through ChainSync, and check
that ChainSync can drain outstanding replies, stop, and restart on the
connection. The restart uses `StopAfterDrain` so the peer receives `MsgDone`
only after every pipelined request has been answered.

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

The `cardano-node-integration` workflow vets and lints the tagged suite, then
runs it against a preview `cardano-node` container started without a Mithril
snapshot, so it serves blocks from genesis shortly after startup. UNIX socket
paths are limited to 108 bytes, so keep the socket path short.

## Leios CertRBs

`TestCardanoNodeInlinedCertRbs` checks that gouroboros can read CertRBs as a
Leios node sends them over node-to-client, with the certified EB's
transactions inlined into the block body. It runs only when
`GOUROBOROS_LEIOS_MIN_CERTRB_BYTES` is also set.

The test syncs from origin to the node's latest block at the time the test
starts.
For every block it checks that:

- the block decodes. For a CertRB with inlined transactions, the decoder
  checks the header's body hash against the body with those transactions
  removed.
- each Dijkstra block passes `ValidateLeiosBlockBodyNodeToClient`: the header's
  certificate flag matches the body's Leios certificate, and the body has no
  Peras certificate.

The test does not run `VerifyBlock` or any ledger validation. In particular,
it does not check that the EB's transactions are applied before the tick to
the CertRB's slot.

The test needs:

- **A Leios prototype node** whose LocalChainSync inlines EB transactions into
  CertRBs. Against an older node, the test fails with "no CertRB with inlined
  transactions on the chain".
- **Transaction load on the devnet.** The test fails unless the largest CertRB
  with inlined transactions is at least `GOUROBOROS_LEIOS_MIN_CERTRB_BYTES`
  bytes, so some EB must be nearly full. An idle devnet fails even when
  nothing is broken.
- **A threshold that matches the devnet.** A full CertRB is roughly the
  devnet's `MaxEndorserBlockTxsSize` protocol parameter plus the header and
  on-chain body. Set the variable just below that. The `12000000` below is an
  example, not a recommended value.

CertRBs with no transactions (an empty EB) are counted and logged, not
failed. At the end, the test logs how many CertRBs it saw and the size and
transaction count of the largest one.

The sync is limited to 30 minutes inside the test, so pass a `go test`
`-timeout` longer than that:

```sh
GOUROBOROS_CARDANO_NODE_SOCKET_PATH=/path/to/node.socket \
GOUROBOROS_CARDANO_NETWORK_MAGIC=42 \
GOUROBOROS_LEIOS_MIN_CERTRB_BYTES=12000000 \
go test -tags=cardano_node_integration -v -count=1 \
  -run TestCardanoNodeInlinedCertRbs -timeout=40m \
  ./internal/test/cardano-node-integration
```
