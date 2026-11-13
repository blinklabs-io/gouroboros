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
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

const (
	// minCertRbBytesEnv opts in to the CertRB test and sets the size of the
	// largest CertRB the chain must contain, so a devnet run proves that a
	// maximum-size CertRB is received.
	minCertRbBytesEnv = "GOUROBOROS_LEIOS_MIN_CERTRB_BYTES"
	certRbSyncLimit   = 30 * time.Minute
)

// syncedCertRb is a CertRB received while syncing.
type syncedCertRb struct {
	slot      uint64
	inlined   bool
	sizeBytes int
	txCount   int
}

// certRbStats summarizes the blocks received while syncing.
type certRbStats struct {
	mu          sync.Mutex
	blocks      uint64
	certRbs     []syncedCertRb
	lastBlockNo uint64
	err         error
}

// rollBack drops the CertRBs after point, so a CertRB from an abandoned fork
// doesn't count toward the checks.
func (s *certRbStats) rollBack(point pcommon.Point) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := len(s.certRbs)
	for keep > 0 && s.certRbs[keep-1].slot > point.Slot {
		keep--
	}
	s.certRbs = s.certRbs[:keep]
}

func (s *certRbStats) record(block any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return
	}
	s.blocks++
	if b, ok := block.(interface{ BlockNumber() uint64 }); ok {
		s.lastBlockNo = b.BlockNumber()
	}
	dijkstraBlock, ok := block.(*dijkstra.DijkstraBlock)
	if !ok || dijkstraBlock.BlockHeader == nil {
		return
	}
	// Check every Dijkstra body, not only CertRBs, so a body with a
	// certificate under an uncertified header is caught.
	if err := dijkstra.ValidateLeiosBlockBodyNodeToClient(
		dijkstraBlock,
	); err != nil {
		s.err = fmt.Errorf(
			"block %d at slot %d: %w",
			dijkstraBlock.BlockNumber(),
			dijkstraBlock.SlotNumber(),
			err,
		)
		return
	}
	if certified, _ := dijkstraBlock.BlockHeader.LeiosCertified(); !certified {
		return
	}
	s.certRbs = append(s.certRbs, syncedCertRb{
		slot:      dijkstraBlock.SlotNumber(),
		inlined:   dijkstraBlock.HasInlinedCertifiedTransactions(),
		sizeBytes: len(dijkstraBlock.Cbor()),
		txCount:   len(dijkstraBlock.BlockBody.Transactions),
	})
}

// TestCardanoNodeInlinedCertRbs syncs a Leios devnet from origin over
// node-to-client with block validation on. Every block must decode: for a
// CertRB with inlined EB transactions, that checks the header's body hash
// against the body without them. The chain must contain such a CertRB of at
// least GOUROBOROS_LEIOS_MIN_CERTRB_BYTES bytes.
func TestCardanoNodeInlinedCertRbs(t *testing.T) {
	socketPath, networkMagic := integrationConfig(t)
	minBytesText := os.Getenv(minCertRbBytesEnv)
	if minBytesText == "" {
		t.Skipf("set %s to run the Leios CertRB test", minCertRbBytesEnv)
	}
	minBytes, err := strconv.Atoi(minBytesText)
	if err != nil || minBytes <= 0 {
		t.Fatalf("invalid %s %q", minCertRbBytesEnv, minBytesText)
	}

	netConn, err := net.DialTimeout("unix", socketPath, 10*time.Second)
	if err != nil {
		t.Fatalf("connect to cardano-node socket %q: %v", socketPath, err)
	}
	stats := &certRbStats{}
	var target uint64
	targetSet := make(chan struct{})
	reached := make(chan struct{})
	var reachedOnce sync.Once
	chainSyncConfig := chainsync.NewConfig(
		chainsync.WithRollBackwardFunc(
			func(
				_ chainsync.CallbackContext,
				point pcommon.Point,
				_ chainsync.Tip,
			) error {
				stats.rollBack(point)
				return nil
			},
		),
		chainsync.WithRollForwardFunc(
			func(
				_ chainsync.CallbackContext,
				_ uint,
				block any,
				_ chainsync.Tip,
			) error {
				stats.record(block)
				<-targetSet
				stats.mu.Lock()
				done := stats.err != nil || stats.lastBlockNo >= target
				stats.mu.Unlock()
				if done {
					reachedOnce.Do(func() { close(reached) })
				}
				return nil
			},
		),
	)
	connectionErrors := make(chan error, 1)
	oConn, err := ouroboros.NewConnection(
		ouroboros.WithConnection(netConn),
		ouroboros.WithNetworkMagic(networkMagic),
		ouroboros.WithNodeToNode(false),
		ouroboros.WithErrorChan(connectionErrors),
		ouroboros.WithChainSyncConfig(chainSyncConfig),
	)
	if err != nil {
		_ = netConn.Close()
		t.Fatalf("start Ouroboros connection: %v", err)
	}
	t.Cleanup(func() { _ = oConn.Close() })
	client := oConn.ChainSync().Client

	tip, err := client.GetCurrentTip()
	if err != nil {
		t.Fatalf("get current chain tip: %v", err)
	}
	target = tip.BlockNumber
	close(targetSet)
	t.Logf("syncing to block %d, slot %d", tip.BlockNumber, tip.Point.Slot)
	if err := client.Sync([]pcommon.Point{pcommon.NewPointOrigin()}); err != nil {
		t.Fatalf("start chain sync from origin: %v", err)
	}

	timer := time.NewTimer(certRbSyncLimit)
	defer timer.Stop()
	select {
	case <-reached:
	case err := <-connectionErrors:
		// A CertRB that fails to decode ends the connection here.
		t.Fatalf("connection failed during chain sync: %v", err)
	case <-timer.C:
		t.Fatalf("timed out syncing to block %d after %s", target, certRbSyncLimit)
	}
	stopCtx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	_ = client.StopAfterDrain(stopCtx)

	stats.mu.Lock()
	defer stats.mu.Unlock()
	if stats.err != nil {
		t.Fatal(stats.err)
	}
	var inlined, withoutTxs, maxBytes, maxTxCount int
	for _, certRb := range stats.certRbs {
		if !certRb.inlined {
			// The node inlines every CertRB's transactions, but an EB can be
			// empty, so this is reported, not failed.
			withoutTxs++
			continue
		}
		inlined++
		if certRb.sizeBytes > maxBytes {
			maxBytes = certRb.sizeBytes
			maxTxCount = certRb.txCount
		}
	}
	t.Logf(
		"%d blocks: %d CertRBs with inlined transactions, %d without; "+
			"largest %d bytes with %d transactions",
		stats.blocks,
		inlined,
		withoutTxs,
		maxBytes,
		maxTxCount,
	)
	if inlined == 0 {
		t.Fatal("no CertRB with inlined transactions on the chain")
	}
	if maxBytes < minBytes {
		t.Fatalf(
			"largest CertRB is %d bytes, want at least %d (%s)",
			maxBytes,
			minBytes,
			minCertRbBytesEnv,
		)
	}
}
