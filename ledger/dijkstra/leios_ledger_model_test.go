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

package dijkstra

import (
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// leiosModelPredecessor is the header a checked block extends, as the
// reference header and body rules distinguish it.
type leiosModelPredecessor int

const (
	leiosPredOrigin leiosModelPredecessor = iota
	leiosPredAnnounces
	leiosPredSilent
	leiosPredPreDijkstra
)

func (p leiosModelPredecessor) String() string {
	return [...]string{"origin", "announces", "silent", "pre-dijkstra"}[p]
}

// leiosModelCell is one block offered to the certificate timing and body
// checks: its predecessor, header flag, body certificate,
// transaction count, slot distance from the predecessor, and whether it
// arrived over node-to-client with its EB's transactions inlined.
type leiosModelCell struct {
	pred         leiosModelPredecessor
	flag         bool
	cert         bool
	txs          int
	gap          uint64
	nodeToClient bool
}

// accepted is the reference acceptance with a valid certificate (CIP-0164
// Ranking Blocks; Dijkstra Rules/Bbody.hs 464-504 and Rules/LeiosCert.hs
// 106-140 at cardano-ledger 1210d37f): the flag
// matches the body, a certificate excludes transactions except when
// inlined over node-to-client, and a flagged header follows an announcing
// predecessor by at least the certification delay. No epoch rule: the
// prototype node gOuroboros follows has none.
func (c leiosModelCell) accepted(delay uint64) bool {
	if c.flag != c.cert {
		return false
	}
	if c.cert && !c.nodeToClient && c.txs > 0 {
		return false
	}
	if c.flag &&
		(c.pred != leiosPredAnnounces || c.gap < delay) {
		return false
	}
	return true
}

// leiosModelCells enumerates every combination of those checks'
// inputs, with slot gaps either side of the certification delay.
func leiosModelCells(delay uint64) []leiosModelCell {
	var cells []leiosModelCell
	for pred := leiosPredOrigin; pred <= leiosPredPreDijkstra; pred++ {
		for _, flag := range []bool{false, true} {
			for _, hasCert := range []bool{false, true} {
				for _, txs := range []int{0, 1} {
					for _, gap := range []uint64{delay - 1, delay, delay + 1} {
						for _, n2c := range []bool{false, true} {
							cells = append(cells, leiosModelCell{
								pred: pred, flag: flag, cert: hasCert,
								txs: txs, gap: gap, nodeToClient: n2c,
							})
						}
					}
				}
			}
		}
	}
	return cells
}

// TestLeiosLedgerModelGoChecks runs every cell through
// ValidateLeiosCertificateTiming and the block body rule together, as
// VerifyBlockState does.
func TestLeiosLedgerModelGoChecks(t *testing.T) {
	t.Parallel()
	pp := testLeiosParams()
	const predSlot = uint64(100)
	delay, err := LeiosMinCertificationSlot(
		time.Second,
		pp.LeiosAnnouncementPeriodLength,
		pp.LeiosVotePeriodLength,
		pp.LeiosDiffusionPeriodLength,
		0,
	)
	require.NoError(t, err)
	require.Greater(t, delay, uint64(1))

	size := uint32(10)
	predecessors := map[leiosModelPredecessor]common.BlockHeader{
		leiosPredOrigin:      nil,
		leiosPredAnnounces:   testLeiosHeader(t, predSlot, 12, false, &size),
		leiosPredSilent:      testLeiosHeader(t, predSlot, 12, false, nil),
		leiosPredPreDijkstra: &babbage.BabbageBlockHeader{},
	}
	cert := &DijkstraLeiosCertificate{
		Signers:             []byte{0x80},
		AggregatedSignature: make([]byte, common.LeiosBlsSignatureSize),
	}

	cells := leiosModelCells(delay)
	accepted := 0
	for _, cell := range cells {
		block := &DijkstraBlock{
			BlockHeader: testLeiosHeader(
				t, predSlot+cell.gap, 12, cell.flag, nil,
			),
		}
		if cell.cert {
			block.BlockBody.LeiosCertificate = cert
		}
		block.BlockBody.Transactions = make([]DijkstraTransaction, cell.txs)
		got := ValidateLeiosCertificateTiming(
			block.BlockHeader, predecessors[cell.pred], pp, time.Second,
		)
		if got == nil && cell.nodeToClient {
			got = ValidateLeiosBlockBodyNodeToClient(block)
		} else if got == nil {
			got = ValidateLeiosBlockBody(block)
		}
		want := cell.accepted(delay)
		require.Equal(t, want, got == nil, "%+v: err %v", cell, got)
		if want {
			accepted++
		}
	}
	require.Len(t, cells, 4*2*2*2*3*2)
	require.Positive(t, accepted)
	require.Less(t, accepted, len(cells))
}
