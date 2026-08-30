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
	"math"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type committeeTermLedgerState struct {
	common.LedgerState
	currentEpoch uint64
}

func (s committeeTermLedgerState) CurrentEpoch() uint64 {
	return s.currentEpoch
}

func committeeCertificateRule(
	t *testing.T,
) common.UtxoValidationRuleFunc {
	t.Helper()
	for _, rule := range conway.UtxoValidationRules {
		name := runtime.FuncForPC(reflect.ValueOf(rule).Pointer()).Name()
		if strings.HasSuffix(
			name,
			"ledger/conway.UtxoValidateCommitteeCertificates",
		) {
			return rule
		}
	}
	t.Fatal("committee certificate rule is not registered")
	return nil
}

func committeeCertificateTx(
	credential common.Credential,
	resign bool,
) *conway.ConwayTransaction {
	var cert common.Certificate
	var certType common.CertificateType
	if resign {
		certType = common.CertificateTypeResignCommitteeCold
		cert = &common.ResignCommitteeColdCertificate{
			CertType:       uint(certType),
			ColdCredential: credential,
		}
	} else {
		certType = common.CertificateTypeAuthCommitteeHot
		cert = &common.AuthCommitteeHotCertificate{
			CertType:       uint(certType),
			ColdCredential: credential,
			HotCredential: common.Credential{
				CredType: common.CredentialTypeAddrKeyHash,
				Credential: common.Blake2b224Hash(
					[]byte("committee-hot-key"),
				),
			},
		}
	}
	return &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxCertificates: []common.CertificateWrapper{{
				Type:        uint(certType),
				Certificate: cert,
			}},
		},
	}
}

func verifyCommitteeCertificate(
	t *testing.T,
	tx common.Transaction,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	t.Helper()
	return common.VerifyTransaction(
		tx,
		0,
		ls,
		pp,
		[]common.UtxoValidationRuleFunc{committeeCertificateRule(t)},
	)
}

func TestCommitteeCertificateTermLimitProductionRule(t *testing.T) {
	coldKey := common.Blake2b224Hash([]byte("committee-cold-key"))
	credential := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: coldKey,
	}

	tests := []struct {
		name         string
		currentEpoch uint64
		expiryEpoch  uint64
		maxTerm      uint64
		proposed     bool
		resign       bool
		wantTooLong  bool
	}{
		{
			name:         "below boundary",
			currentEpoch: 100,
			expiryEpoch:  109,
			maxTerm:      10,
		},
		{
			name:         "at boundary",
			currentEpoch: 100,
			expiryEpoch:  110,
			maxTerm:      10,
		},
		{
			name:         "over boundary",
			currentEpoch: 100,
			expiryEpoch:  111,
			maxTerm:      10,
			wantTooLong:  true,
		},
		{
			name:         "zero limit at current epoch",
			currentEpoch: 100,
			expiryEpoch:  100,
			maxTerm:      0,
			proposed:     true,
		},
		{
			name:         "zero limit with positive remaining term",
			currentEpoch: 100,
			expiryEpoch:  101,
			maxTerm:      0,
			proposed:     true,
			wantTooLong:  true,
		},
		{
			name:         "proposed member resignation over boundary",
			currentEpoch: 100,
			expiryEpoch:  111,
			maxTerm:      10,
			proposed:     true,
			resign:       true,
			wantTooLong:  true,
		},
		{
			name:         "uint64 addition would overflow",
			currentEpoch: math.MaxUint64 - 2,
			expiryEpoch:  math.MaxUint64,
			maxTerm:      5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := mockledger.NewLedgerStateBuilder()
			if tt.proposed {
				builder.WithProposedCommitteeMembers(
					map[common.Blake2b224]uint64{
						coldKey: tt.expiryEpoch,
					},
				)
			} else {
				builder.WithCommitteeMembers([]common.CommitteeMember{{
					ColdKey:     coldKey,
					ExpiryEpoch: tt.expiryEpoch,
				}})
			}
			ls := committeeTermLedgerState{
				LedgerState:  builder.Build(),
				currentEpoch: tt.currentEpoch,
			}
			pp := &conway.ConwayProtocolParameters{
				CommitteeTermLimit: tt.maxTerm,
			}
			err := verifyCommitteeCertificate(
				t,
				committeeCertificateTx(credential, tt.resign),
				ls,
				pp,
			)
			if !tt.wantTooLong {
				require.NoError(t, err)
				return
			}
			var termErr conway.CommitteeTermTooLongError
			require.ErrorAs(t, err, &termErr)
			assert.Equal(t, coldKey, termErr.Credential)
			assert.Equal(t, tt.currentEpoch, termErr.CurrentEpoch)
			assert.Equal(t, tt.expiryEpoch, termErr.ExpiryEpoch)
			assert.Equal(t, tt.maxTerm, termErr.MaxTermLength)
		})
	}
}

func TestCommitteeCertificateTermLimitUnavailable(t *testing.T) {
	coldKey := common.Blake2b224Hash([]byte("committee-cold-key"))
	credential := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: coldKey,
	}
	ls := committeeTermLedgerState{
		LedgerState: mockledger.NewLedgerStateBuilder().
			WithCommitteeMembers([]common.CommitteeMember{{
				ColdKey:     coldKey,
				ExpiryEpoch: 101,
			}}).
			Build(),
		currentEpoch: 100,
	}
	tx := committeeCertificateTx(credential, false)
	var nilPparams *conway.ConwayProtocolParameters

	for name, pp := range map[string]common.ProtocolParameters{
		"unsupported parameters": &mockledger.MockProtocolParamsRules{},
		"typed nil parameters":   nilPparams,
	} {
		t.Run(name, func(t *testing.T) {
			var err error
			require.NotPanics(t, func() {
				err = verifyCommitteeCertificate(t, tx, ls, pp)
			})
			var unavailable conway.CommitteeTermLimitUnavailableError
			require.ErrorAs(t, err, &unavailable)
		})
	}
}

func TestCommitteeCertificateCurrentEpochUnavailable(t *testing.T) {
	coldKey := common.Blake2b224Hash([]byte("committee-cold-key"))
	credential := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: coldKey,
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithCommitteeMembers([]common.CommitteeMember{{
			ColdKey:     coldKey,
			ExpiryEpoch: 101,
		}}).
		Build()
	pp := &conway.ConwayProtocolParameters{CommitteeTermLimit: 10}

	var err error
	require.NotPanics(t, func() {
		err = verifyCommitteeCertificate(
			t,
			committeeCertificateTx(credential, false),
			ls,
			pp,
		)
	})
	var unavailable conway.CurrentEpochStateUnavailableError
	require.ErrorAs(t, err, &unavailable)
}
