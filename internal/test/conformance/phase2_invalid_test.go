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

package conformance_test

import (
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockconformance "github.com/blinklabs-io/ouroboros-mock/conformance"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func TestConformanceValidationRulesSkipDelegationForPhase2Invalid(
	t *testing.T,
) {
	credential := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224{0x01},
	}
	certificates := []common.CertificateWrapper{{
		Type: uint(common.CertificateTypeStakeDelegation),
		Certificate: &common.StakeDelegationCertificate{
			CertType:        uint(common.CertificateTypeStakeDelegation),
			StakeCredential: &credential,
			PoolKeyHash:     common.PoolKeyHash{0x02},
		},
	}}
	state := mockledger.NewLedgerStateBuilder().Build()
	params := &conway.ConwayProtocolParameters{}

	var delegationRule common.UtxoValidationRuleFunc
	delegationPointer := reflect.ValueOf(conway.UtxoValidateDelegation).Pointer()
	for _, rule := range mockconformance.ConformanceValidationRules {
		if reflect.ValueOf(rule).Pointer() == delegationPointer {
			delegationRule = rule
			break
		}
	}
	require.NotNil(t, delegationRule)

	validTx := &conway.ConwayTransaction{
		Body:      conway.ConwayTransactionBody{TxCertificates: certificates},
		TxIsValid: true,
	}
	invalidTx := &conway.ConwayTransaction{
		Body:      conway.ConwayTransactionBody{TxCertificates: certificates},
		TxIsValid: false,
	}

	err := common.VerifyTransaction(
		validTx,
		0,
		state,
		params,
		[]common.UtxoValidationRuleFunc{delegationRule},
	)
	var poolError shelley.DelegateToUnregisteredPoolError
	require.ErrorAs(t, err, &poolError)

	require.NoError(t, common.VerifyTransaction(
		invalidTx,
		0,
		state,
		params,
		[]common.UtxoValidationRuleFunc{delegationRule},
	))
}
