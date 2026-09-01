#!/usr/bin/env bash
#
# Tear down the peer-sharing interop nodes and remove their volumes. Configs
# in ./configs and overlays in ./overlays are kept (delete them by hand to
# force a fresh download next time).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

docker compose -f "${SCRIPT_DIR}/docker-compose.yml" down -v
