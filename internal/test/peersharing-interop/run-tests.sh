#!/usr/bin/env bash
#
# Bring up the peer-sharing interop nodes, run the Go tests against them, and
# tear them down. Use --keep-up to leave the containers running on success
# for poking around.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

KEEP_UP=false
TEST_ARGS=()
for arg in "$@"; do
  case "${arg}" in
    --keep-up) KEEP_UP=true ;;
    *) TEST_ARGS+=("${arg}") ;;
  esac
done

cleanup() {
  local status=$?
  if [[ "${KEEP_UP}" == "true" && ${status} -eq 0 ]]; then
    echo "leaving the network up (--keep-up)"
    return
  fi
  if [[ ${status} -ne 0 ]]; then
    echo "tests failed; dumping the last 100 log lines from each node"
    docker compose -f "${SCRIPT_DIR}/docker-compose.yml" logs --tail=100 || true
  fi
  "${SCRIPT_DIR}/stop.sh" || true
}
trap cleanup EXIT

"${SCRIPT_DIR}/start.sh"

# The healthcheck waits on /ipc/node.socket which cardano-node opens slightly
# before its N2N TCP listener is ready to accept handshakes. Give the listener
# a brief grace window so the first test dial does not race it.
echo "waiting 5s for the TCP listeners to settle..."
sleep 5

export PEERSHARING_SHARES_ON_ADDR="${PEERSHARING_SHARES_ON_ADDR:-localhost:${PEERSHARING_SHARES_ON_PORT:-3010}}"
export PEERSHARING_SHARES_OFF_ADDR="${PEERSHARING_SHARES_OFF_ADDR:-localhost:${PEERSHARING_SHARES_OFF_PORT:-3011}}"

cd "${PROJECT_ROOT}"
TEST_TIMEOUT="${TEST_TIMEOUT:-5m}"
go test -tags peersharing_interop -timeout "${TEST_TIMEOUT}" -count=1 -v \
  "${TEST_ARGS[@]}" ./internal/test/peersharing-interop/...
