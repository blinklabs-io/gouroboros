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

// TestDijkstraCommitteeSharedHotKeySurvivesOtherColdResignation covers the
// dijkstraGovernanceStateView sibling of certs_overlay.go's overlay
// (ledger/conway/certs_overlay.go): GOVCERT's csCommitteeCreds is a Map
// keyed by cold credential (cardano-ledger-core's
// authorizedHotCommitteeCredentials folds every entry into a Set because
// "there is no unique mapping from Hot to Cold credential"), so two cold
// credentials may authorize the same hot credential at once, and resigning
// one must not take voting rights away from the other. This reaches
// dijkstraGovernanceStateView specifically because layer 2
// (conway.UtxoValidateUnknownVoters, invoked once per level) falls back to
// this layer-1 view for any cold credential the current level does not
// itself touch.
func TestDijkstraCommitteeSharedHotKeySurvivesOtherColdResignation(
	t *testing.T,
) {
	t.Parallel()
	coldA := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("dijkstra-shared-hot-cold-a")),
	}
	coldB := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("dijkstra-shared-hot-cold-b")),
	}
	hotHash := common.Blake2b224Hash([]byte("dijkstra-shared-hot-key"))
	// Pin the reverse hot-credential lookup to the untouched cold
	// credential explicitly (see the identical comment in
	// ledger/conway/certs_overlay_gov_test.go): the reference does not
	// define which cold credential a shared hot key resolves to, so the
	// test must not depend on the mock's slice iteration order.
	state := mockledger.NewLedgerStateBuilder().
		WithCommitteeMembers([]common.CommitteeMember{
			{ColdKey: coldA.Credential, HotKey: &hotHash, ExpiryEpoch: 20},
			{ColdKey: coldB.Credential, HotKey: &hotHash, ExpiryEpoch: 20},
		}).
		WithCommitteeHotCredentialMember(
			func(hot common.Credential) (*common.CommitteeMember, error) {
				if hot.CredType != common.CredentialTypeAddrKeyHash ||
					hot.Credential != hotHash {
					return nil, nil
				}
				return &common.CommitteeMember{
					ColdKey:     coldB.Credential,
					HotKey:      &hotHash,
					ExpiryEpoch: 20,
				}, nil
			},
		).
		Build()
	resign := &common.ResignCommitteeColdCertificate{
		CertType:       uint(common.CertificateTypeResignCommitteeCold),
		ColdCredential: coldA,
	}
	voter := &common.Voter{
		Type: common.VoterTypeConstitutionalCommitteeHotKeyHash,
		Hash: hotHash,
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
					TxVotingProcedures: common.VotingProcedures{
						voter: {new(common.GovActionId): {}},
					},
				}},
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

// dijkstraHotMembersLedgerState implements common.CommitteeCredentialState
// and the plural common.CommitteeHotCredentialMembers, modeling a provider
// that has adopted the plural capability: its hotLookup (singular) names
// only the resigned cold credential -- what a singular-only provider's
// reverse index might return -- while hotMembersLookup (plural) returns
// every currently-authorizing cold credential.
type dijkstraHotMembersLedgerState struct {
	common.LedgerState
	coldLookup       func(common.Credential) (*common.CommitteeMember, error)
	hotLookup        func(common.Credential) (*common.CommitteeMember, error)
	hotMembersLookup func(common.Credential) ([]*common.CommitteeMember, error)
}

func (s dijkstraHotMembersLedgerState) CommitteeStateAvailable() (bool, error) {
	return true, nil
}

func (s dijkstraHotMembersLedgerState) CommitteeCredentialMember(
	credential common.Credential,
) (*common.CommitteeMember, error) {
	return s.coldLookup(credential)
}

func (s dijkstraHotMembersLedgerState) CommitteeHotCredentialMember(
	credential common.Credential,
) (*common.CommitteeMember, error) {
	return s.hotLookup(credential)
}

func (s dijkstraHotMembersLedgerState) CommitteeHotCredentialMembers(
	credential common.Credential,
) ([]*common.CommitteeMember, error) {
	return s.hotMembersLookup(credential)
}

// TestDijkstraCommitteeSharedHotKeySurvivesWhenWitnessNamesResignedCold
// mirrors conway's identical test: the dijkstraGovernanceStateView sibling
// must consult the raw ls's plural CommitteeHotCredentialMembers capability
// too, not only the conway overlay layered on top of it, since dingo runs
// this same-transaction resignation through both layers.
func TestDijkstraCommitteeSharedHotKeySurvivesWhenWitnessNamesResignedCold(
	t *testing.T,
) {
	t.Parallel()
	coldA := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("dijkstra-plural-cold-a")),
	}
	coldB := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("dijkstra-plural-cold-b")),
	}
	hotHash := common.Blake2b224Hash([]byte("dijkstra-plural-hot-key"))
	memberFor := func(cold common.Credential) *common.CommitteeMember {
		return &common.CommitteeMember{
			ColdKey: cold.Credential, HotKey: &hotHash, ExpiryEpoch: 20,
		}
	}
	// Both cold credentials are currently seated (enacted committee
	// members), so PV12's inline seated check in UtxoValidateUnknownVoters
	// has a genuine seated candidate to find once cold A is filtered out.
	base := mockledger.NewLedgerStateBuilder().
		WithCommitteeMembers([]common.CommitteeMember{
			*memberFor(coldA),
			*memberFor(coldB),
		}).
		Build()
	state := dijkstraHotMembersLedgerState{
		LedgerState: base,
		coldLookup: func(
			credential common.Credential,
		) (*common.CommitteeMember, error) {
			switch credential.Credential {
			case coldA.Credential:
				return memberFor(coldA), nil
			case coldB.Credential:
				return memberFor(coldB), nil
			default:
				return nil, nil
			}
		},
		hotLookup: func(
			credential common.Credential,
		) (*common.CommitteeMember, error) {
			if credential.Credential != hotHash {
				return nil, nil
			}
			return memberFor(coldA), nil
		},
		hotMembersLookup: func(
			credential common.Credential,
		) ([]*common.CommitteeMember, error) {
			if credential.Credential != hotHash {
				return nil, nil
			}
			return []*common.CommitteeMember{
				memberFor(coldA),
				memberFor(coldB),
			}, nil
		},
	}
	resign := &common.ResignCommitteeColdCertificate{
		CertType:       uint(common.CertificateTypeResignCommitteeCold),
		ColdCredential: coldA,
	}
	voter := &common.Voter{
		Type: common.VoterTypeConstitutionalCommitteeHotKeyHash,
		Hash: hotHash,
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
					TxVotingProcedures: common.VotingProcedures{
						voter: {new(common.GovActionId): {}},
					},
				}},
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

// TestDijkstraCommitteeHotAuthorizationKeepsCredentialType pins the hot
// credential's key/script tag through dijkstraGovernanceStateView: an earlier
// subtransaction authorizing a script-hash hot credential does not make a
// key-hash voter with the same 28 bytes known in a later one, whether cold
// had no hot credential before or had authorized that key-hash credential.
func TestDijkstraCommitteeHotAuthorizationKeepsCredentialType(
	t *testing.T,
) {
	t.Parallel()
	hotHash := common.Blake2b224Hash([]byte("dijkstra-typed-hot-key"))
	testCases := []struct {
		name      string
		priorHot  *common.Blake2b224
		voterType uint8
		known     bool
	}{
		{
			name:      "script authorization, script voter",
			voterType: common.VoterTypeConstitutionalCommitteeHotScriptHash,
			known:     true,
		},
		{
			name:      "script authorization, key voter",
			voterType: common.VoterTypeConstitutionalCommitteeHotKeyHash,
		},
		{
			name:      "key authorization replaced by script, key voter",
			priorHot:  &hotHash,
			voterType: common.VoterTypeConstitutionalCommitteeHotKeyHash,
		},
	}
	cold := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("dijkstra-typed-hot-cold")),
	}
	authorize := common.CertificateWrapper{
		Type: uint(common.CertificateTypeAuthCommitteeHot),
		Certificate: &common.AuthCommitteeHotCertificate{
			CertType:       uint(common.CertificateTypeAuthCommitteeHot),
			ColdCredential: cold,
			HotCredential: common.Credential{
				CredType:   common.CredentialTypeScriptHash,
				Credential: hotHash,
			},
		},
	}
	params := &DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: common.ProtocolVersionDijkstra,
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state := mockledger.NewLedgerStateBuilder().WithCommitteeMembers(
				[]common.CommitteeMember{{
					ColdKey:     cold.Credential,
					HotKey:      tc.priorHot,
					ExpiryEpoch: 20,
				}},
			).Build()
			voter := &common.Voter{Type: tc.voterType, Hash: hotHash}
			subTransactions := []DijkstraSubTransaction{
				{Body: DijkstraSubTransactionBody{
					TxCertificates: []common.CertificateWrapper{authorize},
				}},
				{Body: DijkstraSubTransactionBody{
					TxVotingProcedures: common.VotingProcedures{
						voter: {new(common.GovActionId): {}},
					},
				}},
			}
			tx := &DijkstraTransaction{
				TxIsValid: true,
				Body: DijkstraTransactionBody{
					TxSubTransactions: cbor.NewSetType(subTransactions, true),
				},
			}

			err := UtxoValidateUnknownVoters(tx, 0, state, params)
			if tc.known {
				require.NoError(t, err)
				return
			}
			require.ErrorAs(t, err, &conway.UnknownVoterError{})
		})
	}
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
