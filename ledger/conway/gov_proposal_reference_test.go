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

package conway_test

import (
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// rewardAddressOnNetwork builds a key-hash reward account on the given
// network: header 0xE0 | network id.
func rewardAddressOnNetwork(
	t *testing.T,
	network byte,
	keyHash common.Blake2b224,
) common.Address {
	t.Helper()
	addr, err := common.NewAddressFromBytes(
		append([]byte{0xE0 | network}, keyHash.Bytes()...),
	)
	require.NoError(t, err)
	return addr
}

// TestGovProposalNetworkIdReferenceTable walks every combination of a
// proposal's return-account network and, for a TreasuryWithdrawals action,
// its withdrawal-account networks. cardano-ledger
// eras/conway/impl/src/Cardano/Ledger/Conway/Rules/Gov.hs (1210d37f) lines
// 532-546 raise ProposalProcedureNetworkIdMismatch when the return account's
// network is not the node's, and TreasuryWithdrawalsNetworkIdMismatch when
// any withdrawal account's network is not; other action kinds carry no
// accounts. A proposal is accepted only when every account is on the node's
// network.
func TestGovProposalNetworkIdReferenceTable(t *testing.T) {
	const ledgerNetwork = 1
	ls := mockledger.NewLedgerStateBuilder().WithNetworkId(ledgerNetwork).Build()
	pp := mkConwayPp(common.ProtocolVersionPlomin, 0)
	for _, returnNet := range []byte{0, 1} {
		for _, kind := range []string{"info", "withdrawal"} {
			for _, withdrawalNets := range [][]byte{{1}, {0}, {1, 0}, {1, 1}} {
				if kind == "info" && len(withdrawalNets) > 1 {
					continue
				}
				name := fmt.Sprintf(
					"return=%d/%s/withdrawals=%v",
					returnNet,
					kind,
					withdrawalNets,
				)
				t.Run(name, func(t *testing.T) {
					want := returnNet == ledgerNetwork
					var action common.GovAction = &common.InfoGovAction{}
					if kind == "withdrawal" {
						withdrawals := map[*common.Address]uint64{}
						for i, net := range withdrawalNets {
							addr := rewardAddressOnNetwork(
								t,
								net,
								common.Blake2b224{0x20, byte(i)},
							)
							withdrawals[&addr] = 1
							if net != ledgerNetwork {
								want = false
							}
						}
						action = &common.TreasuryWithdrawalGovAction{
							Withdrawals: withdrawals,
						}
					}
					tx := mkProposalTx(
						0,
						rewardAddressOnNetwork(t, returnNet, common.Blake2b224{0x10}),
						action,
					)
					err := conway.UtxoValidateProposalNetworkIds(tx, 0, ls, pp)
					if want {
						require.NoError(t, err)
						return
					}
					var mismatch conway.WrongNetworkProposalAddressError
					require.ErrorAs(t, err, &mismatch)
				})
			}
		}
	}
}

// TestGovProposalReturnAddressShapeReferenceTable checks that only a reward
// account can be a proposal's return address. cardano-ledger types
// pProcReturnAddr as AccountAddress
// (eras/conway/impl/src/Cardano/Ledger/Conway/Governance/Procedures.hs), so
// a proposal whose return address is any other address kind does not decode.
func TestGovProposalReturnAddressShapeReferenceTable(t *testing.T) {
	hash := common.Blake2b224{0x30}.Bytes()
	cases := []struct {
		name   string
		header byte
		body   []byte
		accept bool
	}{
		{"reward key", 0xE1, hash, true},
		{"reward script", 0xF1, hash, true},
		{"base key/key", 0x01, append(append([]byte{}, hash...), hash...), false},
		{"enterprise key", 0x61, hash, false},
		{"enterprise script", 0x71, hash, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addr, err := common.NewAddressFromBytes(
				append([]byte{tc.header}, tc.body...),
			)
			require.NoError(t, err)
			tx := mkProposalTx(0, addr, &common.InfoGovAction{})
			err = common.UtxoValidateProposalReturnAddressShape(tx, 0, nil, nil)
			if tc.accept {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
