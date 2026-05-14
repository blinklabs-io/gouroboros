# Peer Sharing Interop

A minimal Docker Compose harness that runs two Haskell `cardano-node`
containers — one with `PeerSharing=true`, one with `PeerSharing=false` — and a
matching Go test suite (build tag `peersharing_interop`) that connects to each
node and verifies that gouroboros's peer-sharing handshake gating matches what
the remote advertised.

This covers the "bidirectional handshake negotiation" acceptance criterion of
issue #1703 against a real reference implementation.

## What it verifies

Two scenarios, both run against the same gouroboros code path
(`ouroboros.NewConnection` → handshake → `oConn.PeerSharing().Client.GetPeers`):

| Test | Remote advertised | Expected outcome |
|---|---|---|
| `TestRemoteAdvertisesPeerSharing` | `PeerSharing=true` | Handshake `VersionData.PeerSharing()` is `true`; `GetPeers` succeeds (slice may be empty). |
| `TestRemoteDisablesPeerSharing` | `PeerSharing=false` | Handshake `VersionData.PeerSharing()` is `false`; `GetPeers` returns `peersharing.ErrRemotePeerSharingDisabled` and sends no bytes on the wire. |

The local-disabled side (server refusing inbound `ShareRequest` when this node
advertised `NoPeerSharing`) cannot be exercised from this initiator-only test:
gouroboros is the client of the handshake here, not a remote-reachable server.
That path is covered by the unit test in
[`protocol/peersharing/server_test.go`](../../../protocol/peersharing/server_test.go).

## Topology

| Container          | PeerSharing | Host port (default)                                | Container port |
|--------------------|-------------|----------------------------------------------------|----------------|
| `cardano-shares-on`  | `true`    | `${PEERSHARING_SHARES_ON_PORT:-3010}`              | `3001`         |
| `cardano-shares-off` | `false`   | `${PEERSHARING_SHARES_OFF_PORT:-3011}`             | `3001`         |

Both run the upstream preview-network configs but with an overlay that:

- Forces `EnableP2P = true` (peer sharing is only defined in the P2P stack).
- Sets `PeerSharing` as a JSON **boolean** per-node. (cardano-node's parser
  is strict here — it errors on the string enum form.)
- Replaces the topology with empty local- and public-root sets, so the node
  does not attempt to dial outbound and the test does not depend on
  preview-net connectivity beyond fetching genesis files on first run.

The node will still start, load genesis, and listen on its port; it just sits
idle, which is exactly what we want — the test only exercises the handshake
and peer-sharing mini-protocol, not chain sync.

## Prerequisites

- Docker with the Compose plugin (`docker compose version` must work).
- `jq` on the host (used by `fetch-configs.sh` to produce overlay config.json).
- `curl` on the host (used by `fetch-configs.sh` to fetch upstream configs).
- Go (matching `go.mod`) on the host to run the integration tests.
- Outbound HTTPS to `book.world.dev.cardano.org` on first run, to download
  the preview-net genesis files. Subsequent runs reuse the cached copies
  under `./configs/`.

## Running

`run-tests.sh` is the entry point used both locally and in CI:

```bash
# from this directory
./run-tests.sh                                       # full cycle
./run-tests.sh -run TestRemoteAdvertisesPeerSharing  # forward args to go test
./run-tests.sh --keep-up                             # leave nodes running on success

# from the repo root
./internal/test/peersharing-interop/run-tests.sh
```

What it does:

1. `fetch-configs.sh` downloads (if missing) the upstream preview configs into
   `./configs/`, then materialises two overlay directories in `./overlays/`
   (one per node) that point at `/configs/*` inside the container and set the
   correct `PeerSharing` flag.
2. `docker compose up -d` brings both nodes up; the script polls Docker's
   healthcheck (`test -S /ipc/node.socket`) for up to 180s.
3. From the repo root, runs:
   ```
   go test -tags peersharing_interop -count=1 -v ./internal/test/peersharing-interop/...
   ```
4. Tears the network down. On failure it dumps the last 100 log lines of each
   container first.

Manual usage:

```bash
./start.sh        # fetch configs + docker compose up -d + wait for healthy
./stop.sh         # docker compose down -v (removes the DB and IPC volumes)
```

## Environment overrides

| Variable                          | Default                                     | Used by |
|-----------------------------------|---------------------------------------------|---------|
| `PEERSHARING_SHARES_ON_PORT`      | `3010`                                      | docker-compose host port for `cardano-shares-on` |
| `PEERSHARING_SHARES_OFF_PORT`     | `3011`                                      | docker-compose host port for `cardano-shares-off` |
| `PEERSHARING_SHARES_ON_ADDR`      | `localhost:${PEERSHARING_SHARES_ON_PORT}`   | go test target for the shares-on node |
| `PEERSHARING_SHARES_OFF_ADDR`     | `localhost:${PEERSHARING_SHARES_OFF_PORT}`  | go test target for the shares-off node |
| `PEERSHARING_NODE_IMAGE`          | `ghcr.io/blinklabs-io/cardano-node:11.0.1`  | docker-compose image (matches the pin in dingo/internal/test/devnet) |
| `PEERSHARING_CONFIG_BASE_URL`     | `https://book.world.dev.cardano.org/environments/preview` | `fetch-configs.sh` base URL |
| `TEST_TIMEOUT`                    | `5m`                                        | `go test -timeout` in `run-tests.sh` |

Pointing `PEERSHARING_SHARES_*_ADDR` at a different host lets you reuse the
test against any running cardano-node — e.g. a preview-net relay you control
— without bringing up the Docker Compose stack at all. Skip `start.sh` /
`run-tests.sh` and invoke `go test` directly in that case:

```bash
PEERSHARING_SHARES_ON_ADDR=preview-relay.example:3001 \
PEERSHARING_SHARES_OFF_ADDR=another-relay.example:3001 \
  go test -tags peersharing_interop -v ./internal/test/peersharing-interop/...
```

## Files

| File | Purpose |
|------|---------|
| `docker-compose.yml` | Two `cardano-node` services + per-node DB and IPC volumes |
| `fetch-configs.sh`   | Downloads upstream preview configs and builds the per-node overlay (`config.json` + empty-root `topology.json`) under `./overlays/{shares-on,shares-off}/` |
| `start.sh`           | `fetch-configs.sh` → `docker compose up -d` → wait for healthy |
| `stop.sh`            | `docker compose down -v` |
| `run-tests.sh`       | Full bring-up → `go test -tags peersharing_interop` → tear-down |
| `harness_test.go`    | The two scenarios above. Build-tagged so it does not run as part of the normal `go test ./...`. |

## Cleanup

`stop.sh` and the `run-tests.sh` trap both run `docker compose down -v`,
removing the per-node DB and IPC volumes. The downloaded configs in
`./configs/` and the materialised overlays in `./overlays/` are kept; delete
them by hand to force a fresh download of upstream genesis on the next run.

## Troubleshooting

- **`fetch-configs.sh` exits with "jq is required".** Install `jq` on the host
  (`apt install jq` / `brew install jq`).
- **Nodes never become healthy.** Check `docker compose logs cardano-shares-on`
  / `cardano-shares-off`. Two common causes:
    - **Logs show `mithril-client cardano-db download latest`.** The blinklabs
      image entrypoint defaults to downloading a full mainnet Mithril snapshot
      on first run. Both services in `docker-compose.yml` set
      `RESTORE_SNAPSHOT: "false"` and `CARDANO_NETWORK: preview` to suppress
      this; if you copied the compose file without those env vars, add them.
    - **Logs show `touch: cannot touch '/overlay/config.json': Read-only file system`.**
      This is **expected and required**. The image's `/usr/local/bin/run-node`
      entrypoint sed-rewrites `"PeerSharing": false` → `"PeerSharing": true`
      for any non-block-producer node whose config file is writable. The
      overlay is therefore bind-mounted `:ro`, the touch test fails, and
      the rewrite is skipped (the touch is wrapped in `set +e` so the error
      message is benign and the node still starts). If you accidentally
      drop `:ro`, the `shares-off` node will silently advertise peer
      sharing and `TestRemoteDisablesPeerSharing` will fail with
      "shares-off node should advertise NoPeerSharing".
    - **Logs show `CardanoProtocolInstantiationCheckpointsReadError ".../<something>.json"`.**
      The upstream preview `config.json` references additional files (today
      `checkpoints.json`) as paths relative to the config. We rewrite the
      genesis paths to `/configs/*` and `del()` the `CheckpointsFile` /
      `CheckpointsFileHash` keys, but if upstream adds a new relative path
      we have not stripped, the node fails the same way. Add a matching
      `del(.NewField)` line to `build_overlay()` in `fetch-configs.sh`.
    - **Upstream config schema drift in another field.** If none of the above
      applies, the preview config we fetch may have changed in a way the
      empty-roots topology no longer satisfies. Wipe both layers:
      `rm -rf configs overlays && ./start.sh`.
- **`port is already allocated`.** Override `PEERSHARING_SHARES_ON_PORT` /
  `PEERSHARING_SHARES_OFF_PORT`.
- **`tcp dial localhost:3010: connection refused`.** The container is up but
  the node hasn't bound the listener yet. The healthcheck waits on the IPC
  socket which appears slightly before the TCP listener; if `run-tests.sh`
  is racing the listener, add `sleep 5` after `start.sh` in the script — or
  re-run; `go test` will retry the dial.
