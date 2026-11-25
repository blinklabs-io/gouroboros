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

package ledger_test

import (
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// TestVerifyBlockStateConwayHeaderProtVerTooHigh follows the Conway BBODY
// rule: on mainnet, a header's major protocol version may exceed the one in
// force by at most one. Other networks have no upper bound.
func TestVerifyBlockStateConwayHeaderProtVerTooHigh(t *testing.T) {
	const current = 10
	for _, network := range []uint{
		common.AddressNetworkMainnet,
		common.AddressNetworkTestnet,
	} {
		for _, headerMajor := range []uint64{
			current - 1,
			current,
			current + 1,
			current + 2,
			current + 3,
		} {
			wantReject := network == common.AddressNetworkMainnet &&
				headerMajor > current+1
			name := fmt.Sprintf("network %d header %d", network, headerMajor)
			t.Run(name, func(t *testing.T) {
				block := &conway.ConwayBlock{
					BlockHeader: &conway.ConwayBlockHeader{
						BabbageBlockHeader: babbage.BabbageBlockHeader{
							Body: babbage.BabbageBlockHeaderBody{
								ProtoVersion: babbage.BabbageProtoVersion{
									Major: headerMajor,
								},
							},
						},
					},
				}
				pp := &conway.ConwayProtocolParameters{}
				pp.ProtocolVersion.Major = current
				err := ledger.VerifyBlockState(block, common.VerifyConfig{
					SkipBlockLimitsValidation: true,
					SkipTransactionValidation: true,
					SkipStakePoolValidation:   true,
					ProtocolParameters:        pp,
					LedgerState: mockledger.NewLedgerStateBuilder().
						WithNetworkId(network).
						Build(),
				})
				if !wantReject {
					require.NoError(t, err)
					return
				}
				var tooHigh *conway.HeaderProtVerTooHighError
				require.ErrorAs(t, err, &tooHigh)
				require.Equal(t, headerMajor, tooHigh.Supplied)
				require.Equal(t, uint64(current+1), tooHigh.Expected)
			})
		}
	}
}
