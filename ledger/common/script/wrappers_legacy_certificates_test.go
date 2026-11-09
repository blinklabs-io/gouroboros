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

package script_test

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

func TestTxInfoV1V2LegacyCertificates(t *testing.T) {
	credential := &common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224{0x42},
	}
	certificates := []struct {
		name        string
		certificate common.Certificate
		want        data.PlutusData
	}{
		{
			name: "genesis delegation",
			certificate: &common.GenesisKeyDelegationCertificate{
				GenesisHash:         []byte{0x01},
				GenesisDelegateHash: []byte{0x02},
				VrfKeyHash:          common.VrfKeyHash{0x03},
			},
			want: data.NewConstr(5),
		},
		{
			name: "MIR rewards",
			certificate: &common.MoveInstantaneousRewardsCertificate{
				Reward: common.MoveInstantaneousRewardsCertificateReward{
					Rewards: map[*common.Credential]*big.Int{
						credential: big.NewInt(1),
					},
				},
			},
			want: data.NewConstr(6),
		},
		{
			name: "MIR pot transfer",
			certificate: &common.MoveInstantaneousRewardsCertificate{
				Reward: common.MoveInstantaneousRewardsCertificateReward{
					OtherPot: 1,
				},
			},
			want: data.NewConstr(6),
		},
	}
	contexts := []struct {
		name       string
		txType     int
		fieldIndex int
		build      func(common.Transaction) (script.TxInfo, error)
	}{
		{
			name: "Alonzo V1", txType: 4, fieldIndex: 4,
			build: func(tx common.Transaction) (script.TxInfo, error) {
				return script.NewTxInfoV1FromTransaction(nil, tx, nil, false, 5)
			},
		},
		{
			name: "Babbage V1", txType: 5, fieldIndex: 4,
			build: func(tx common.Transaction) (script.TxInfo, error) {
				return script.NewTxInfoV1FromTransaction(nil, tx, nil, false, 7)
			},
		},
		{
			name: "Babbage V2", txType: 5, fieldIndex: 5,
			build: func(tx common.Transaction) (script.TxInfo, error) {
				return script.NewTxInfoV2FromTransaction(nil, tx, nil, false, 7)
			},
		},
	}
	for _, certificate := range certificates {
		for _, context := range contexts {
			t.Run(certificate.name+"/"+context.name, func(t *testing.T) {
				tx := mockledger.NewTransactionBuilder().
					WithType(context.txType).
					WithCertificates(certificate.certificate)
				info, err := context.build(tx)
				require.NoError(t, err)
				fields := info.ToPlutusData().(*data.Constr).Fields()
				translated := fields[context.fieldIndex].(*data.List)
				require.Equal(
					t,
					[]data.PlutusData{certificate.want},
					translated.Items(),
				)
				require.Empty(t, translated.Items()[0].(*data.Constr).Fields())
			})
		}
	}
}
