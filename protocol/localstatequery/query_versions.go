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

package localstatequery

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// shelleyQueryVersionRange bounds the node-to-client versions that carry a
// Shelley query: first is the earliest, last (when non-zero) the latest.
type shelleyQueryVersionRange struct {
	first uint16
	last  uint16
}

// shelleyQueryVersionRanges follows blockQueryIsSupportedOnVersion in
// ouroboros-consensus, translated from Shelley node-to-client versions to
// the node-to-client versions they ride on (12 on 20, 13 on 21, 15 on 23).
// The node closes the connection on a query its version does not carry, so
// both sides refuse it before it reaches the wire or the callback.
var shelleyQueryVersionRanges = map[int]shelleyQueryVersionRange{
	QueryTypeShelleyProposedProtocolParamsUpdates: {last: 19},
	QueryTypeShelleyStakeDistribution:             {last: 20},
	QueryTypeShelleyPoolDistr:                     {last: 20},
	QueryTypeShelleyStakePoolDefaultVote:          {first: 20},
	QueryTypeShelleyPoolDistr2:                    {first: 21},
	QueryTypeShelleyStakeDistribution2:            {first: 21},
	QueryTypeShelleyMaxMajorProtocolVersion:       {first: 21},
	QueryTypeShelleyDRepDelegations:               {first: 23},
}

// shelleyQuerySupported reports whether the node-to-client protocol version
// (with its handshake offset) carries the Shelley query type.
func shelleyQuerySupported(queryType int, version uint16) bool {
	r, ok := shelleyQueryVersionRanges[queryType]
	if !ok {
		return true
	}
	// Wraps for an unset version, which the version flags elsewhere in this
	// package also read as "newest".
	v := version - protocol.ProtocolVersionNtCOffset
	if v < r.first {
		return false
	}
	return r.last == 0 || v <= r.last
}

func unsupportedQueryError(name string, version uint16) error {
	return fmt.Errorf(
		"%w: %s on node-to-client version %d",
		ErrQueryUnsupportedVersion,
		name,
		version-protocol.ProtocolVersionNtCOffset,
	)
}

// shelleyLeafQuery returns the decoded Shelley query inside the block, era,
// and any GetCBOR wrappers.
func shelleyLeafQuery(w QueryWrapper) any {
	bq, ok := w.Query.(*BlockQuery)
	if !ok {
		return nil
	}
	sq, ok := bq.Query.(*ShelleyQuery)
	if !ok {
		return nil
	}
	query := sq.Query
	for {
		cborQuery, isCbor := query.(*ShelleyCborQuery)
		if !isCbor {
			return query
		}
		query = cborQuery.Query
	}
}

// shelleyLeafQueryType returns the Shelley query type of a decoded query
// wrapper, if it holds one whose availability depends on the version.
func shelleyLeafQueryType(w QueryWrapper) (int, bool) {
	query := shelleyLeafQuery(w)
	switch query.(type) {
	case *ShelleyProposedProtocolParamsUpdatesQuery:
		return QueryTypeShelleyProposedProtocolParamsUpdates, true
	case *ShelleyStakeDistributionQuery:
		return QueryTypeShelleyStakeDistribution, true
	case *ShelleyPoolDistrQuery:
		return QueryTypeShelleyPoolDistr, true
	case *ShelleyStakePoolDefaultVoteQuery:
		return QueryTypeShelleyStakePoolDefaultVote, true
	case *ShelleyPoolDistr2Query:
		return QueryTypeShelleyPoolDistr2, true
	case *ShelleyStakeDistribution2Query:
		return QueryTypeShelleyStakeDistribution2, true
	case *ShelleyMaxMajorProtocolVersionQuery:
		return QueryTypeShelleyMaxMajorProtocolVersion, true
	case *ShelleyDRepDelegationsQuery:
		return QueryTypeShelleyDRepDelegations, true
	}
	return 0, false
}
