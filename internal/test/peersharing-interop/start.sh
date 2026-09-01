#!/usr/bin/env bash
#
# Bring up the peer-sharing interop nodes. Idempotent: re-running with the
# configs already cached just calls `docker compose up -d`.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"${SCRIPT_DIR}/fetch-configs.sh"

docker compose -f "${SCRIPT_DIR}/docker-compose.yml" up -d

echo "waiting for both nodes to become healthy..."
# 5 min: cardano-node initialising on an empty DB can take 60-90s before it
# binds the socket, and we have two of them coming up in parallel on the
# same host.
deadline=$(( $(date +%s) + 300 ))
while :; do
  on_state=$(docker inspect -f '{{.State.Health.Status}}' cardano-shares-on 2>/dev/null || echo missing)
  off_state=$(docker inspect -f '{{.State.Health.Status}}' cardano-shares-off 2>/dev/null || echo missing)
  echo "  shares-on=${on_state} shares-off=${off_state}"
  if [[ "${on_state}" == "healthy" && "${off_state}" == "healthy" ]]; then
    echo "both nodes healthy"
    exit 0
  fi
  if (( $(date +%s) >= deadline )); then
    echo "timed out waiting for nodes to become healthy" >&2
    docker compose -f "${SCRIPT_DIR}/docker-compose.yml" ps
    docker compose -f "${SCRIPT_DIR}/docker-compose.yml" logs --tail=80
    exit 1
  fi
  sleep 5
done
