---
name: dingo-block-producer-operator
description: Operate and troubleshoot an existing Dingo Cardano block producer, including Docker or Kubernetes lifecycle actions, forging and sync health checks, KES/opcert rotation, HA safety, and incident evidence. Use for running or recovering a deployed producer; do not use for Dingo or dingo-operator source changes, tests, architecture, or generated manifests, which belong to dingo-maintainer or infrastructure-reviewer.
---

# Dingo Block Producer Operator

Use this skill for an already-built and deployed Dingo block producer. Establish
the deployment mode first, preserve the producer's keys and database, and prove
forging from more than one signal before declaring it healthy.

## Scope and safety boundary

- This skill may inspect and operate processes, containers, StatefulSets, PVCs,
  Secrets, metrics, logs, and topology. It does not edit Dingo or operator
  source, CRDs, Helm templates, generated manifests, or architecture docs.
- Never print, paste, or commit key contents. Use Secret names, file metadata,
  hashes produced by an approved tool, and redacted log excerpts instead.
- Cold pool keys stay outside the cluster. Rotation may deliver a KES key, VRF
  key, and operational certificate, but cold signing remains an external
  workflow.
- Treat a producer as testnet/devnet unless the current Dingo release
  documentation explicitly supports the target network. The Dingo README has
  historically stated that current releases do not support mainnet operation.

## Operating workflow

1. **Identify the live unit.** Record the network and magic, pool ID, Dingo
   image/version and revision, deployment mode, namespace/container, data path
   or PVC, config bundle, keys Secret, topology peers, HA strategy, and the
   current tip. For Kubernetes, inspect the `DingoNode`, StatefulSet, pod,
   PVC, Events, and relevant Services before changing anything.

2. **Check the boot contract.** Confirm the three producer files exist without
   reading them: `vrf.skey`, `kes.skey`, and `opcert.cert`. They must be mounted
   as files with restrictive permissions and readable by the Dingo runtime user
   (the operator-managed image uses UID 100/GID 101). Confirm that the network
   config and every genesis file named by `config.json` are present. A custom
   generated devnet needs its generated `config.json` and sibling genesis
   files; a named network may use the image's bundled config.

3. **Start, stop, or restart conservatively.** Prefer the deployment's native
   lifecycle action and watch termination, startup, and readiness to completion.
   For an operator-managed producer, a valid Secret change should produce a
   checksum change and StatefulSet rollout; use `rollout status` and pod Events
   to follow it. Do not force-delete a pod during first-boot genesis creation.
   Dingo versions before 0.68.0 can permanently brick a reused PVC after an
   interrupted genesis write; check the image tag before treating a startup
   CrashLoop as recoverable.

4. **Prove sync and forging.** Readiness only proves the process is responsive.
   Check logs for successful block production and advancing slots/blocks, query
   the node's tip, and compare two observations over a bounded interval. Check
   Prometheus for current KES period, remaining KES periods, opcert start
   period, and forged-block counters. Also verify peer freshness and topology;
   a node can be healthy while syncing from an ineligible or stale peer.

5. **Handle rotation as a guarded rollout.** `RotationDue` is driven by
   remaining KES periods and the configured renewal threshold. A new bundle
   must be complete and internally matched: the opcert's cold signature and
   pool binding, the KES public key, the VRF key, the counter, and the current
   KES window must all be valid. Replace the Secret atomically, observe the
   operator's validation condition and Event, then verify the rollout and
   forging again. Never assume that “no rollout” means the rejected files are
   absent: a whole-Secret volume can refresh them onto the running pod even
   while the process keeps its previously loaded keys. Fix or remove rejected
   material promptly before any unrelated restart.

6. **Triage before repairing.** Classify the failure as process/config, key or
   certificate validation, sync/peer selection, storage/PVC, resource pressure,
   topology/network policy, Mithril bootstrap, or HA fencing. Preserve the
   first failing logs, pod/container state, image digest, config and topology
   identity, tip/slot observations, peer list, metrics, and rollout history.
   For a sync plateau, check chain-selection eligibility, peer freshness,
   intersection history, and stalled-client recycling before changing data.

7. **Hand off with evidence.** Report the exact target, timestamps, commands
   and exit codes, observations before and after the action, remaining risk, and
   anything skipped. Redact key values, tokens, kubeconfigs, and private
   endpoints. If the evidence points to a Dingo or operator defect, stop at
   evidence collection and switch to `dingo-maintainer` or
   `infrastructure-reviewer` for implementation.

## Detailed runbook

Load [producer-operations.md](references/producer-operations.md) for the
deployment-mode command matrix, forging signals, rotation checklist, and
failure-specific evidence requirements.
