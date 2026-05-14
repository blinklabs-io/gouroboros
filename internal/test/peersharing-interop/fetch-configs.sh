#!/usr/bin/env bash
#
# Download the upstream preview-network configs once, then materialise two
# overlay directories (./overlays/shares-on, ./overlays/shares-off) whose
# config.json differs only by the PeerSharing flag. Topology is forced to
# empty local roots so the node does not attempt to sync the chain — we only
# need its handshake + peer-sharing surface to be reachable.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIGS_DIR="${SCRIPT_DIR}/configs"
OVERLAYS_DIR="${SCRIPT_DIR}/overlays"

CONFIG_BASE_URL="${PEERSHARING_CONFIG_BASE_URL:-https://book.world.dev.cardano.org/environments/preview}"
CONFIG_FILES=(
  config.json
  byron-genesis.json
  shelley-genesis.json
  alonzo-genesis.json
  conway-genesis.json
)

mkdir -p "${CONFIGS_DIR}"

# Fetch the base configs once. They are cached on disk; delete the directory
# to force a refresh.
for f in "${CONFIG_FILES[@]}"; do
  if [[ ! -f "${CONFIGS_DIR}/${f}" ]]; then
    echo "fetching ${f}"
    curl -fsSL -o "${CONFIGS_DIR}/${f}" "${CONFIG_BASE_URL}/${f}"
  fi
done

# Always regenerate overlays from the current configs + this script's logic.
# The base configs in ./configs/ stay cached; only the per-node overlay files
# are rebuilt so changes to build_overlay (e.g. new fields to strip) take
# effect on the next start without needing to wipe ./configs/.
rm -rf "${OVERLAYS_DIR}"

if ! command -v jq >/dev/null 2>&1; then
  echo "error: jq is required to patch the cardano-node config" >&2
  exit 1
fi

# Build a config.json overlay for each peer-sharing setting. We start from the
# upstream preview config and:
#   - force EnableP2P (peer sharing is only meaningful in P2P mode)
#   - set PeerSharing as a JSON boolean per the function argument. The
#     cardano-node parser explicitly expects Boolean here ("expected Boolean,
#     but encountered String" if you try the enum form).
#   - rewrite genesis paths to /configs (bind-mounted from ./configs)
#   - drop CheckpointsFile / CheckpointsFileHash so the node does not try to
#     read a checkpoints file we did not fetch. Checkpoints are a chain-
#     validation aid and irrelevant for our peer-sharing-only handshake test;
#     stripping them avoids having to track yet another upstream artefact.
#
# IMPORTANT: the overlay must be bind-mounted READ-ONLY in docker-compose.
# The image's /usr/local/bin/run-node entrypoint sed-rewrites
# `"PeerSharing": false` → `"PeerSharing": true` for any non-block-producer
# node whose config file is writable. The :ro mount makes the touch-test
# fail and the rewrite gets skipped (under set +e, so the touch error
# message is harmless and the script continues).
build_overlay() {
  local name="$1" peer_sharing="$2"
  local dir="${OVERLAYS_DIR}/${name}"
  mkdir -p "${dir}"

  jq --argjson ps "${peer_sharing}" '
      .EnableP2P = true
    | .PeerSharing = $ps
    | .ByronGenesisFile = "/configs/byron-genesis.json"
    | .ShelleyGenesisFile = "/configs/shelley-genesis.json"
    | .AlonzoGenesisFile = "/configs/alonzo-genesis.json"
    | .ConwayGenesisFile = "/configs/conway-genesis.json"
    | del(.CheckpointsFile)
    | del(.CheckpointsFileHash)
  ' "${CONFIGS_DIR}/config.json" > "${dir}/config.json"

  # Empty local roots, no public roots, no DNS bootstrap. The node will start,
  # open its listener, and answer handshakes without ever dialling outbound.
  cat > "${dir}/topology.json" <<'JSON'
{
  "localRoots": [
    { "accessPoints": [], "advertise": false, "trustable": false, "valency": 0 }
  ],
  "publicRoots": [
    { "accessPoints": [], "advertise": false }
  ],
  "useLedgerAfterSlot": -1
}
JSON
}

build_overlay shares-on  true
build_overlay shares-off false

echo "configs ready in ${CONFIGS_DIR}"
echo "overlays ready in ${OVERLAYS_DIR}"
