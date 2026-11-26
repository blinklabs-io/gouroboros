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
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// GOVCERT accepts a hot-key authorization or a resignation from a cold
// credential that a pending UpdateCommittee proposal would add
// (isPotentialFutureMember in cardano-ledger 1210d37fe6bd
// eras/conway/impl/src/Cardano/Ledger/Conway/Rules/GovCert.hs). A
// sub-transaction's certificates see the proposals of the levels before it,
// and the top level those of every sub-transaction (Dijkstra
// Rules/SubLedger.hs 259-299, Rules/Ledger.hs 430-478); a level's own
// proposal comes after its certificates and does not count.
func TestDijkstraCommitteeCertificateForEarlierLevelProposedMember(
	t *testing.T,
) {
	credential := func(s string) common.Credential {
		return common.Credential{
			CredType:   common.CredentialTypeAddrKeyHash,
			Credential: common.Blake2b224Hash([]byte(s)),
		}
	}
	seated := credential("proposed-member-seated")
	proposed := credential("proposed-member-new")
	hot := credential("proposed-member-hot")
	state := dijkstraCommitteeVotingState{
		LedgerState: mockledger.NewLedgerStateBuilder().WithCommitteeMembers(
			[]common.CommitteeMember{{ColdKey: seated.Credential, ExpiryEpoch: 20}},
		).Build(),
	}
	params := &DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: common.ProtocolVersionDijkstra,
			},
		},
	}
	proposal := []DijkstraProposalProcedure{{
		PPGovAction: DijkstraGovAction{
			Action: &common.UpdateCommitteeGovAction{
				Type: uint(common.GovActionTypeUpdateCommittee),
				CredEpochs: map[*common.Credential]uint64{
					&proposed: 30,
				},
				Quorum: cbor.Rat{Rat: big.NewRat(1, 2)},
			},
		},
	}}
	authorize := []common.CertificateWrapper{{
		Type: uint(common.CertificateTypeAuthCommitteeHot),
		Certificate: &common.AuthCommitteeHotCertificate{
			CertType:       uint(common.CertificateTypeAuthCommitteeHot),
			ColdCredential: proposed,
			HotCredential:  hot,
		},
	}}
	resign := []common.CertificateWrapper{{
		Type: uint(common.CertificateTypeResignCommitteeCold),
		Certificate: &common.ResignCommitteeColdCertificate{
			CertType:       uint(common.CertificateTypeResignCommitteeCold),
			ColdCredential: proposed,
		},
	}}
	batch := func(
		subs []DijkstraSubTransactionBody,
		top DijkstraTransactionBody,
	) *DijkstraTransaction {
		built := make([]DijkstraSubTransaction, 0, len(subs))
		for idx := range subs {
			built = append(built, DijkstraSubTransaction{Body: subs[idx]})
		}
		if len(built) > 0 {
			top.TxSubTransactions = cbor.NewSetType(built, true)
		}
		return &DijkstraTransaction{TxIsValid: true, Body: top}
	}

	for _, tc := range []struct {
		name  string
		tx    *DijkstraTransaction
		known bool
	}{
		{
			name: "top authorizes a member a sub-transaction proposed",
			tx: batch(
				[]DijkstraSubTransactionBody{{TxProposalProcedures: proposal}},
				DijkstraTransactionBody{TxCertificates: authorize},
			),
			known: true,
		},
		{
			name: "later sub-transaction resigns a member an earlier one proposed",
			tx: batch(
				[]DijkstraSubTransactionBody{
					{TxProposalProcedures: proposal},
					{TxCertificates: resign},
				},
				DijkstraTransactionBody{},
			),
			known: true,
		},
		{
			name: "a level's own proposal does not count",
			tx: batch(
				[]DijkstraSubTransactionBody{
					{TxProposalProcedures: proposal, TxCertificates: authorize},
				},
				DijkstraTransactionBody{},
			),
		},
		{
			name: "a later level's proposal does not count",
			tx: batch(
				[]DijkstraSubTransactionBody{{TxCertificates: authorize}},
				DijkstraTransactionBody{TxProposalProcedures: proposal},
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := UtxoValidateCommitteeCertificates(tc.tx, 0, state, params)
			if tc.known {
				require.NoError(t, err)
				return
			}
			require.ErrorAs(t, err, &conway.NotCommitteeMemberError{})
		})
	}
}
