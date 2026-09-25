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

type dijkstraEpochState struct {
	common.LedgerState
	epoch uint64
}

func (s dijkstraEpochState) EpochForSlot(uint64) (uint64, error) {
	return s.epoch, nil
}

func TestDijkstraCommitteeAuthorizationCarriesAcrossSubtransactions(
	t *testing.T,
) {
	cold := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("committee-cold")),
	}
	hot := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("committee-hot")),
	}
	state := mockledger.NewLedgerStateBuilder().WithCommitteeMembers(
		[]common.CommitteeMember{{ColdKey: cold.Credential, ExpiryEpoch: 20}},
	).Build()
	authorize := &common.AuthCommitteeHotCertificate{
		CertType:       uint(common.CertificateTypeAuthCommitteeHot),
		ColdCredential: cold,
		HotCredential:  hot,
	}
	voter := &common.Voter{
		Type: common.VoterTypeConstitutionalCommitteeHotKeyHash,
		Hash: hot.Credential,
	}
	tx := &DijkstraTransaction{
		TxIsValid: true,
		Body: DijkstraTransactionBody{
			TxSubTransactions: cbor.NewSetType([]DijkstraSubTransaction{
				{
					Body: DijkstraSubTransactionBody{
						TxCertificates: []common.CertificateWrapper{{
							Type:        uint(common.CertificateTypeAuthCommitteeHot),
							Certificate: authorize,
						}},
					},
				},
				{
					Body: DijkstraSubTransactionBody{
						TxVotingProcedures: common.VotingProcedures{
							voter: {new(common.GovActionId): {}},
						},
					},
				},
			}, true),
		},
	}
	params := &DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: common.ProtocolVersionDijkstra,
			},
		},
	}

	require.NoError(t, UtxoValidateUnknownVoters(tx, 0, state, params))
}

func TestDijkstraCommitteeResignationCarriesAcrossSubtransactions(
	t *testing.T,
) {
	cold := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("committee-resign-cold")),
	}
	state := mockledger.NewLedgerStateBuilder().WithCommitteeMembers(
		[]common.CommitteeMember{{ColdKey: cold.Credential, ExpiryEpoch: 20}},
	).Build()
	resign := &common.ResignCommitteeColdCertificate{
		CertType:       uint(common.CertificateTypeResignCommitteeCold),
		ColdCredential: cold,
	}
	authorize := &common.AuthCommitteeHotCertificate{
		CertType:       uint(common.CertificateTypeAuthCommitteeHot),
		ColdCredential: cold,
		HotCredential: common.Credential{
			CredType:   common.CredentialTypeAddrKeyHash,
			Credential: common.Blake2b224Hash([]byte("committee-resign-hot")),
		},
	}
	tx := &DijkstraTransaction{
		TxIsValid: true,
		Body: DijkstraTransactionBody{
			TxSubTransactions: cbor.NewSetType([]DijkstraSubTransaction{
				{Body: DijkstraSubTransactionBody{
					TxCertificates: []common.CertificateWrapper{{
						Type:        uint(common.CertificateTypeResignCommitteeCold),
						Certificate: resign,
					}},
				}},
				{Body: DijkstraSubTransactionBody{
					TxCertificates: []common.CertificateWrapper{{
						Type:        uint(common.CertificateTypeAuthCommitteeHot),
						Certificate: authorize,
					}},
				}},
			}, true),
		},
	}
	err := UtxoValidateCommitteeCertificates(
		tx,
		0,
		state,
		&DijkstraProtocolParameters{},
	)
	require.ErrorAs(t, err, &conway.ResignedCommitteeMemberHotKeyError{})
}

func TestDijkstraProposalProceduresRejectExpiredCommitteeMembers(
	t *testing.T,
) {
	credential := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("expired-committee")),
	}
	state := dijkstraEpochState{
		LedgerState: mockledger.NewLedgerStateBuilder().Build(),
		epoch:       4,
	}
	for _, test := range []struct {
		name        string
		expiry      uint64
		wantExpired bool
	}{
		{name: "current epoch is expired", expiry: 4, wantExpired: true},
		{name: "future epoch is accepted", expiry: 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			action := &common.UpdateCommitteeGovAction{
				Type: uint(common.GovActionTypeUpdateCommittee),
				CredEpochs: map[*common.Credential]uint64{
					&credential: test.expiry,
				},
				Quorum: cbor.Rat{Rat: big.NewRat(1, 2)},
			}
			tx := &DijkstraTransaction{
				TxIsValid: true,
				Body: DijkstraTransactionBody{
					TxSubTransactions: cbor.NewSetType([]DijkstraSubTransaction{{
						Body: DijkstraSubTransactionBody{
							TxProposalProcedures: []DijkstraProposalProcedure{{
								PPGovAction: DijkstraGovAction{Action: action},
							}},
						},
					}}, true),
				},
			}

			err := UtxoValidateProposalProcedures(
				tx,
				0,
				state,
				&DijkstraProtocolParameters{},
			)
			if !test.wantExpired {
				require.NoError(t, err)
				return
			}
			var expired conway.CommitteeMemberAlreadyExpiredError
			require.ErrorAs(t, err, &expired)
			require.Equal(t, test.expiry, expired.ExpiryEpoch)
			require.Equal(t, uint64(4), expired.CurrentEpoch)
		})
	}
}
