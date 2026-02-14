#!/bin/bash
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

# profile.sh - Generate and analyze pprof profiles for gouroboros components
#
# Usage:
#   ./scripts/profile.sh [component] [profile_type] [options]
#
# Components:
#   block     - Block validation profiling (default)
#   cbor      - CBOR decode profiling
#   tx        - Transaction validation profiling
#   vrf       - VRF operations profiling
#   consensus - Consensus operations profiling
#   bodyhash  - Body hash validation profiling
#   script    - Native script evaluation profiling
#   all       - Run all profiling tests
#
# Profile types:
#   cpu       - CPU profile only
#   mem       - Memory profile only
#   both      - Both CPU and memory (default)
#
# Options:
#   --view    - Open profiles in pprof web UI after generation
#   --help    - Show this help message
#
# Examples:
#   ./scripts/profile.sh block cpu
#   ./scripts/profile.sh cbor mem --view
#   ./scripts/profile.sh all both
#   ./scripts/profile.sh vrf both --view

set -e

# Default values
COMPONENT="block"
PROFILE_TYPE="both"
VIEW_PROFILES=false
OUTPUT_DIR="profiles"

# Parse all arguments (options and positional) in a single pass
POSITIONAL=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        --view)
            VIEW_PROFILES=true
            shift
            ;;
        --help|-h)
            sed -n '16,44p' "$0" | sed 's/^# //' | sed 's/^#//'
            exit 0
            ;;
        -*)
            echo "Unknown option: $1"
            exit 1
            ;;
        *)
            POSITIONAL+=("$1")
            shift
            ;;
    esac
done

# Assign positional arguments
if [[ ${#POSITIONAL[@]} -ge 1 ]]; then
    COMPONENT="${POSITIONAL[0]}"
fi
if [[ ${#POSITIONAL[@]} -ge 2 ]]; then
    PROFILE_TYPE="${POSITIONAL[1]}"
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Map component names to test names
component_to_test() {
    case "$1" in
        block)     echo "TestProfileBlockValidation" ;;
        cbor)      echo "TestProfileCBORDecode" ;;
        tx)        echo "TestProfileTxValidation" ;;
        vrf)       echo "TestProfileVRF" ;;
        consensus) echo "TestProfileConsensus" ;;
        bodyhash)  echo "TestProfileBodyHash" ;;
        script)    echo "TestProfileNativeScript" ;;
        *)         echo "" ;;
    esac
}

# Run profiling for a single component
run_profile() {
    local component="$1"
    local test_name
    test_name=$(component_to_test "$component")

    if [[ -z "$test_name" ]]; then
        echo "Unknown component: $component"
        return 1
    fi

    echo "=== Profiling $component ($test_name) ==="

    local cpu_prof="$OUTPUT_DIR/cpu_${component}.prof"
    local mem_prof="$OUTPUT_DIR/mem_${component}.prof"
    local cmd_args="-tags=profile -run=$test_name -timeout=10m"

    case "$PROFILE_TYPE" in
        cpu)
            echo "Generating CPU profile: $cpu_prof"
            go test $cmd_args -cpuprofile="$cpu_prof" ./internal/bench/...
            ;;
        mem)
            echo "Generating memory profile: $mem_prof"
            go test $cmd_args -memprofile="$mem_prof" ./internal/bench/...
            ;;
        both)
            echo "Generating CPU profile: $cpu_prof"
            echo "Generating memory profile: $mem_prof"
            go test $cmd_args -cpuprofile="$cpu_prof" -memprofile="$mem_prof" ./internal/bench/...
            ;;
        *)
            echo "Unknown profile type: $PROFILE_TYPE"
            return 1
            ;;
    esac

    echo "Profile(s) saved to $OUTPUT_DIR/"
    echo ""
}

# View profiles in web UI
view_profiles() {
    local component="$1"
    local cpu_prof="$OUTPUT_DIR/cpu_${component}.prof"
    local mem_prof="$OUTPUT_DIR/mem_${component}.prof"
    local port=8080

    if [[ "$PROFILE_TYPE" == "cpu" || "$PROFILE_TYPE" == "both" ]] && [[ -f "$cpu_prof" ]]; then
        echo "Opening CPU profile in browser at http://localhost:$port"
        go tool pprof -http=localhost:$port "$cpu_prof" &
        port=$((port + 1))
    fi

    if [[ "$PROFILE_TYPE" == "mem" || "$PROFILE_TYPE" == "both" ]] && [[ -f "$mem_prof" ]]; then
        echo "Opening memory profile in browser at http://localhost:$port"
        go tool pprof -http=localhost:$port "$mem_prof" &
    fi

    echo ""
    echo "Press Ctrl+C to stop the pprof servers"
    wait
}

# Main execution
if [[ "$COMPONENT" == "all" ]]; then
    COMPONENTS=(block cbor tx vrf consensus bodyhash script)
    for comp in "${COMPONENTS[@]}"; do
        run_profile "$comp" || true
    done

    if [[ "$VIEW_PROFILES" == true ]]; then
        echo "Note: --view with 'all' only opens the first component's profiles"
        view_profiles "block"
    fi
else
    run_profile "$COMPONENT"

    if [[ "$VIEW_PROFILES" == true ]]; then
        view_profiles "$COMPONENT"
    fi
fi

echo "=== Profiling complete ==="
echo ""
if [[ "$COMPONENT" == "all" ]]; then
    echo "To analyze profiles manually (example using 'block'):"
    echo "  go tool pprof -http=localhost:8080 $OUTPUT_DIR/cpu_block.prof"
    echo "  go tool pprof -http=localhost:8081 $OUTPUT_DIR/mem_block.prof"
    echo ""
    echo "Available components: block cbor tx vrf consensus bodyhash script"
else
    echo "To analyze profiles manually:"
    echo "  go tool pprof -http=localhost:8080 $OUTPUT_DIR/cpu_${COMPONENT}.prof"
    echo "  go tool pprof -http=localhost:8081 $OUTPUT_DIR/mem_${COMPONENT}.prof"
fi
echo ""
echo "For text-based analysis:"
echo "  go tool pprof -top $OUTPUT_DIR/cpu_\${COMPONENT}.prof"
echo "  go tool pprof -top $OUTPUT_DIR/mem_\${COMPONENT}.prof"
echo ""
echo "Compare two profiles:"
echo "  go tool pprof -base=old.prof new.prof"
