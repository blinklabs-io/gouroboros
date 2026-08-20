# Dingo producer operations reference

Use placeholders such as <namespace>, <name>, <pod>, <keys-secret>, and
<config-path>. Never substitute secret contents into commands or tickets.

## Establish the target

Determine whether the producer is a local process, a Docker Compose service, or
a Kubernetes DingoNode managed by dingo-operator. Capture the network and
magic, pool ID, image digest, config identity, data location, topology, and
active/standby identity.

For Kubernetes, inspect before changing anything:

~~~sh
kubectl -n <namespace> get dingonode <name> -o yaml
kubectl -n <namespace> get statefulset,pod,pvc,service -l app.kubernetes.io/instance=<name> -o wide
kubectl -n <namespace> get events --sort-by=.lastTimestamp
kubectl -n <namespace> describe pod <pod>
~~~

If labels differ, select children from the DingoNode owner references rather
than guessing. Record the exact image tag and digest before a restart.

## Start and stop patterns

For a direct process, keep the network and key paths explicit:

~~~sh
CARDANO_NETWORK=<network> \
CARDANO_BLOCK_PRODUCER=true \
CARDANO_SHELLEY_VRF_KEY=<vrf-path> \
CARDANO_SHELLEY_KES_KEY=<kes-path> \
CARDANO_SHELLEY_OPERATIONAL_CERTIFICATE=<opcert-path> \
./dingo
~~~

For the Dingo development network, use its lifecycle scripts and inspect logs:

~~~sh
cd <dingo-repo>/internal/test/devnet
./start.sh
docker compose -f docker-compose.yml ps
docker compose -f docker-compose.yml logs --no-color <producer-service>
./stop.sh
~~~

The devnet is a lab environment. It generates fresh genesis and pool material,
uses one-second slots and short epochs, and is not a public-network deployment.

For an operator-managed producer, prefer the operator rollout path:

~~~sh
kubectl -n <namespace> rollout status statefulset/<name> --timeout=10m
kubectl -n <namespace> get pods -l app.kubernetes.io/instance=<name> -o wide
kubectl -n <namespace> logs <pod> --since=15m --timestamps
~~~

Use kubectl rollout restart only when the current keys/config are known-good
and a restart is intended. Never delete the PVC as a first response to a
CrashLoop.

## Boot and key checks

Check metadata, not values:

~~~sh
kubectl -n <namespace> get secret <keys-secret> -o jsonpath='{.data}' | jq 'keys'
kubectl -n <namespace> get secret <keys-secret> -o jsonpath='{.metadata.resourceVersion}'
~~~

Inside a controlled diagnostic shell, check file metadata without reading keys:

~~~sh
stat -c '%a %u:%g %n' /keys/vrf.skey /keys/kes.skey /keys/opcert.cert
~~~

The operator path expects cardano-cli text-envelope files and a Dingo runtime
that can read mode 0600 files as UID 100/GID 101. A certificate from the wrong
pool, an opcert paired with the wrong KES key, a stale counter, or a missing
genesis sibling is a boot failure, not a reason to weaken checks.

For a generated custom network, inspect config references without exposing keys:

~~~sh
jq -r 'to_entries[] | select(.key|test("GenesisFile$")) | .value' <config-path>/config.json
~~~

Every listed path must resolve beside config.json, and network magic must match
the genesis and Dingo configuration.

## Forging and sync verification

Use at least one log signal and one metric/tip signal, separated in time:

~~~sh
kubectl -n <namespace> logs <pod> --since=10m --timestamps \
  | rg 'block produced|block_number|chain extended|VRF|KES|opcert'
curl -fsS http://<metrics-address>:12798/metrics \
  | rg 'currentKESPeriod|remainingKESPeriods|operationalCertificateStartKESPeriod|Forge_forged'
~~~

Metric names may be prefixed or normalized by the scrape path; match semantic
names rather than assuming one exporter spelling. Record timestamp A and B for
the tip slot/block number, forged-block counter, current and remaining KES
period, and selected-peer freshness/connection state.

“Ready”, an open port, or nonzero uptime is not forging proof. Check for VRF key
hash mismatch, KES verification-key mismatch, expired or future opcert, and
repeated database/genesis errors in the same log window.

If node-to-client local-state-query is enabled for on-chain opcert counters,
verify both sides: Dingo must listen on node-to-client and the client
pod/namespace must carry the allowed label. A policy drop often appears as a
timeout, not an application error.

## Human-assisted rotation

1. Confirm RotationDue and capture current KES/opcert status.
2. Generate the new KES material and certificate outside the cluster; obtain the
   cold signature through the approved signer.
3. Confirm the new KES key matches the opcert, the pool binding is correct, and
   the counter is not below the authoritative on-chain value. Do not increment
   blindly from a local file.
4. Apply the complete Secret update atomically. Avoid a partial bundle.
5. Watch KeysValid, Degraded, acceptance/rejection Events, the keys-checksum
   annotation, ControllerRevision, and pod rollout.
6. Repeat the forging proof and confirm exactly one HA member is active.

An accepted bundle changes the pod-template checksum and rolls the StatefulSet.
A rejected bundle should not initiate a rollout, but a whole-Secret volume may
still refresh on disk. Restore a known-good Secret before any unrelated
restart, eviction, drain, or reschedule.

## Failure triage

| Symptom | Check first | Preserve |
|---|---|---|
| CrashLoop after key update | envelope kind, mode/UID, KES-opcert match, pool ID, counter, KES window | Secret resource versions, logs, image digest |
| VRF key hash mismatch | VRF key belongs to the selected pool registration | redacted config identity, pool ID, file metadata |
| Sync stalls or tip plateaus | eligible/fresh peers, chain selection, intersection, stalled clients | tip/slot timeline, peer list, warnings |
| NtC counter timeout | listener, port 3002, NetworkPolicy labels | policy objects, endpoints, dial timestamps |
| Mithril init never completes | aggregator/network and init-container logs | init status, image, endpoint, progress |
| FK error after early restart | Dingo tag and interrupted genesis write | full startup log and PVC identity; do not wipe first |
| Two active producers | HA lease/fencing and key exposure | pod identity, lease, readiness, mount metadata |

For the pre-0.68.0 genesis failure, the PVC may be unrecoverable without a wipe
after upgrading. Treat wiping as an explicit data-loss recovery decision, not
routine cleanup.

## Evidence handoff

Record UTC timestamps, command exit codes, deployment revision, image digest,
network/config identity, tip and forging samples, KES/opcert status, peer state,
Events, and the exact action taken. Include skipped checks and why. Redact
Secret data, cold-key material, credentials, kubeconfig contents, and private
addresses unless explicitly authorized.
