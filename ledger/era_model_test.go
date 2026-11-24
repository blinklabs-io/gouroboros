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

package ledger

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

// eraVersionRow is one era with gOuroboros' protocol-version range and the
// reference's ProtVerLow and ProtVerHigh (cardano-ledger 1210d37fe6bd,
// libs/cardano-ledger-core/internal/Cardano/Ledger/Internal/Definition/
// Era.hs). The low version is also the default hard-fork trigger
// (ouroboros-consensus-cardano Cardano/Node.hs,
// CardanoTriggerHardForkAtDefaultVersion).
type eraVersionRow struct {
	era             common.Era
	goMin, goMax    uint
	refLow, refHigh uint
}

func eraVersionTable() []eraVersionRow {
	return []eraVersionRow{
		{shelley.EraShelley, shelley.MinProtocolVersionShelley, shelley.MaxProtocolVersionShelley, 2, 2},
		{allegra.EraAllegra, allegra.MinProtocolVersionAllegra, allegra.MaxProtocolVersionAllegra, 3, 3},
		{mary.EraMary, mary.MinProtocolVersionMary, mary.MaxProtocolVersionMary, 4, 4},
		{alonzo.EraAlonzo, alonzo.MinProtocolVersionAlonzo, alonzo.MaxProtocolVersionAlonzo, 5, 6},
		{babbage.EraBabbage, babbage.MinProtocolVersionBabbage, babbage.MaxProtocolVersionBabbage, 7, 8},
		{conway.EraConway, conway.MinProtocolVersionConway, conway.MaxProtocolVersionConway, 9, 11},
		{dijkstra.EraDijkstra, dijkstra.MinProtocolVersionDijkstra, dijkstra.MaxProtocolVersionDijkstra, 12, 12},
	}
}

// eraForVersion is the era a ledger at protocol major version major is in,
// given the reference's ProtVerLow values (Era.hs above): the last era whose
// lowest version is at or below major.
func eraForVersion(major uint, lows func(eraVersionRow) uint) (common.Era, bool) {
	var found common.Era
	ok := false
	for _, row := range eraVersionTable() {
		if lows(row) <= major {
			found, ok = row.era, true
		}
	}
	return found, ok
}

// TestEraProtocolVersionsMatchReference checks each era's lowest protocol
// version against the reference, and that the ranges tile the versions
// with no gap or overlap, so every major names exactly one era.
func TestEraProtocolVersionsMatchReference(t *testing.T) {
	rows := eraVersionTable()
	for i, row := range rows {
		require.Equal(t, row.refLow, row.goMin, "%s lowest version", row.era.Name)
		require.LessOrEqual(t, row.goMin, row.goMax, "%s range", row.era.Name)
		if row.era != dijkstra.EraDijkstra {
			require.Equal(t, row.refHigh, row.goMax, "%s highest version", row.era.Name)
		} else {
			// gOuroboros places PV13 (the phased-in minimum pool margin rule)
			// in Dijkstra; the pinned reference's Dijkstra stops at 12.
			require.GreaterOrEqual(t, row.goMax, row.refHigh)
		}
		if i+1 < len(rows) {
			require.Equal(t, row.goMax+1, rows[i+1].goMin,
				"%s and %s ranges must meet", row.era.Name, rows[i+1].era.Name)
		}
	}
	for major := uint(2); major <= 12; major++ {
		goEra, goOk := eraForVersion(major, func(r eraVersionRow) uint { return r.goMin })
		refEra, refOk := eraForVersion(major, func(r eraVersionRow) uint { return r.refLow })
		require.Equal(t, refOk, goOk, "major %d", major)
		require.Equal(t, refEra, goEra, "major %d", major)
	}
}

// TestNamedProtocolVersionsLieInTheirEra checks the hard-fork constants in
// ledger/common against the era each one belongs to. Alonzo 6 and Babbage 8
// are each era's second (intra-era) version, not the version that starts
// the era; ProtoMajor* in verify_block.go are the starting versions.
func TestNamedProtocolVersionsLieInTheirEra(t *testing.T) {
	low := func(r eraVersionRow) uint { return r.refLow }
	tests := []struct {
		major uint
		era   common.Era
	}{
		{common.ProtocolVersionShelley, shelley.EraShelley},
		{common.ProtocolVersionAllegra, allegra.EraAllegra},
		{common.ProtocolVersionMary, mary.EraMary},
		{common.ProtocolVersionAlonzo, alonzo.EraAlonzo},
		{common.ProtocolVersionBabbage, babbage.EraBabbage},
		{common.ProtocolVersionConway, conway.EraConway},
		{common.ProtocolVersionPlomin, conway.EraConway},
		{common.ProtocolVersionVanRossem, conway.EraConway},
		{common.ProtocolVersionDijkstra, dijkstra.EraDijkstra},
		{ProtoMajorShelley, shelley.EraShelley},
		{ProtoMajorAllegra, allegra.EraAllegra},
		{ProtoMajorMary, mary.EraMary},
		{ProtoMajorAlonzo, alonzo.EraAlonzo},
		{ProtoMajorBabbage, babbage.EraBabbage},
		{ProtoMajorConway, conway.EraConway},
		{ProtoMajorDijkstra, dijkstra.EraDijkstra},
	}
	for _, test := range tests {
		got, ok := eraForVersion(test.major, low)
		require.True(t, ok, "major %d", test.major)
		require.Equal(t, test.era, got, "major %d", test.major)
	}
	starts := map[common.Era]uint{
		shelley.EraShelley:   ProtoMajorShelley,
		allegra.EraAllegra:   ProtoMajorAllegra,
		mary.EraMary:         ProtoMajorMary,
		alonzo.EraAlonzo:     ProtoMajorAlonzo,
		babbage.EraBabbage:   ProtoMajorBabbage,
		conway.EraConway:     ProtoMajorConway,
		dijkstra.EraDijkstra: ProtoMajorDijkstra,
	}
	for _, row := range eraVersionTable() {
		require.Equal(t, row.refLow, starts[row.era], "%s start version", row.era.Name)
	}
}

// TestEraOrderFollowsHardForkSequence checks that eraOrder ranks the eras
// in the order the hard forks happen, which is also era id order, and that
// eraAtLeast agrees with it for every pair.
func TestEraOrderFollowsHardForkSequence(t *testing.T) {
	eras := []common.Era{byron.EraByron}
	for _, row := range eraVersionTable() {
		eras = append(eras, row.era)
	}
	for i, era := range eras {
		order, ok := eraOrder(era)
		require.True(t, ok, era.Name)
		require.Equal(t, i, order, era.Name)
		require.Equal(t, uint8(i), era.Id, era.Name)
		for j, other := range eras {
			require.Equal(t, i >= j, eraAtLeast(era, other), "%s >= %s", era.Name, other.Name)
		}
	}
	_, ok := eraOrder(common.Era{Id: 200, Name: "unknown"})
	require.False(t, ok)
	require.False(t, eraAtLeast(common.Era{Id: 200, Name: "unknown"}, byron.EraByron))
}
