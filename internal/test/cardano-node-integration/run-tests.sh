#!/usr/bin/env bash
# Copyright 2026 Blink Labs Software
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

repo_root=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
if [[ -z "${GOUROBOROS_CARDANO_NODE_SOCKET_PATH:-}" ]]; then
  printf '%s\n' \
    'Set GOUROBOROS_CARDANO_NODE_SOCKET_PATH to the node UNIX socket.' >&2
  exit 1
fi
if [[ -z "${GOUROBOROS_CARDANO_NETWORK_MAGIC:-}" ]]; then
  printf '%s\n' \
    'Set GOUROBOROS_CARDANO_NETWORK_MAGIC to the node network magic.' >&2
  exit 1
fi

cd "$repo_root"
GOWORK=off go test \
  -tags=cardano_node_integration \
  -count=1 \
  -timeout=5m \
  -v \
  ./internal/test/cardano-node-integration
