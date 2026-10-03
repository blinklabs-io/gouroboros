// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build cardano_node_integration

package cardanonodeintegration

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

const (
	socketPathEnv   = "GOUROBOROS_CARDANO_NODE_SOCKET_PATH"
	networkMagicEnv = "GOUROBOROS_CARDANO_NETWORK_MAGIC"
	blockWaitLimit  = 3 * time.Minute
)

type rollForwardResult struct {
	block lcommon.Block
	err   error
}

func integrationConfig(t *testing.T) (string, uint32) {
	t.Helper()
	socketPath := os.Getenv(socketPathEnv)
	networkMagicText := os.Getenv(networkMagicEnv)
	if socketPath == "" || networkMagicText == "" {
		t.Skipf(
			"set %s and %s to run cardano-node integration tests",
			socketPathEnv,
			networkMagicEnv,
		)
	}
	networkMagic, err := strconv.ParseUint(networkMagicText, 10, 32)
	if err != nil {
		t.Fatalf("invalid %s %q: %v", networkMagicEnv, networkMagicText, err)
	}
	return socketPath, uint32(networkMagic)
}

func dialNode(
	t *testing.T,
	socketPath string,
	networkMagic uint32,
) (*ouroboros.Connection, <-chan error, <-chan rollForwardResult) {
	t.Helper()

	netConn, err := net.DialTimeout("unix", socketPath, 10*time.Second)
	if err != nil {
		t.Fatalf("connect to cardano-node socket %q: %v", socketPath, err)
	}

	rollForward := make(chan rollForwardResult, 1)
	chainSyncConfig := chainsync.NewConfig(
		chainsync.WithRollForwardFunc(
			func(
				_ chainsync.CallbackContext,
				_ uint,
				data any,
				_ chainsync.Tip,
			) error {
				block, ok := data.(lcommon.Block)
				if !ok {
					select {
					case rollForward <- rollForwardResult{
						err: fmt.Errorf("expected a decoded block, got %T", data),
					}:
					default:
					}
					return nil
				}
				select {
				case rollForward <- rollForwardResult{block: block}:
				default:
				}
				return nil
			},
		),
	)
	errors := make(chan error, 1)
	oConn, err := ouroboros.NewConnection(
		ouroboros.WithConnection(netConn),
		ouroboros.WithNetworkMagic(networkMagic),
		ouroboros.WithNodeToNode(false),
		ouroboros.WithErrorChan(errors),
		ouroboros.WithChainSyncConfig(chainSyncConfig),
	)
	if err != nil {
		_ = netConn.Close()
		t.Fatalf("start Ouroboros connection: %v", err)
	}
	t.Cleanup(func() {
		_ = oConn.Close()
	})
	return oConn, errors, rollForward
}

func TestCardanoNodeChainSync(t *testing.T) {
	socketPath, networkMagic := integrationConfig(t)
	oConn, connectionErrors, rollForward := dialNode(t, socketPath, networkMagic)
	client := oConn.ChainSync().Client

	tip, err := client.GetCurrentTip()
	if err != nil {
		t.Fatalf("get current chain tip: %v", err)
	}
	if len(tip.Point.Hash) != 32 {
		t.Fatalf("expected a nonempty chain tip hash, got tip %#v", tip)
	}
	t.Logf(
		"connected to chain tip at block %d, slot %d",
		tip.BlockNumber,
		tip.Point.Slot,
	)

	if err := client.Sync([]pcommon.Point{pcommon.NewPointOrigin()}); err != nil {
		t.Fatalf("start chain sync from origin: %v", err)
	}

	timer := time.NewTimer(blockWaitLimit)
	defer timer.Stop()
	select {
	case result := <-rollForward:
		if result.err != nil {
			t.Fatal(result.err)
		}
		if len(result.block.Hash().Bytes()) != 32 || len(result.block.Cbor()) == 0 {
			t.Fatalf(
				"invalid decoded block: era %v, slot %d",
				result.block.Era(),
				result.block.SlotNumber(),
			)
		}
		t.Logf(
			"decoded %v block %d at slot %d",
			result.block.Era(),
			result.block.BlockNumber(),
			result.block.SlotNumber(),
		)
	case err := <-connectionErrors:
		t.Fatalf("connection failed during chain sync: %v", err)
	case <-timer.C:
		t.Fatalf(
			"timed out waiting for a block from cardano-node after %s",
			blockWaitLimit,
		)
	}

	if err := client.Stop(); err != nil {
		t.Fatalf("stop chain sync client: %v", err)
	}
	client.Start()
	restartedTip, err := client.GetCurrentTip()
	if err != nil {
		t.Fatalf("get chain tip after restarting chain sync: %v", err)
	}
	if len(restartedTip.Point.Hash) != 32 {
		t.Fatalf("expected a chain tip after restarting, got %#v", restartedTip)
	}
}
