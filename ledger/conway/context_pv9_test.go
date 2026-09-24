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
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

func TestTxInfoV3FromTransactionPreservesCertificateAmountRules(t *testing.T) {
	credential := common.Credential{Credential: common.Blake2b224{1}}
	amount := big.NewInt(2_000_000)
	tests := []struct {
		name        string
		certificate common.Certificate
		want        map[uint]data.PlutusData
	}{
		{
			name: "explicit registration",
			certificate: &common.RegistrationCertificate{
				StakeCredential: credential,
				Amount:          amount.Int64(),
			},
			want: map[uint]data.PlutusData{
				common.ProtocolVersionConway:   data.NewConstr(1),
				common.ProtocolVersionPlomin:   data.NewConstr(0, data.NewInteger(amount)),
				common.ProtocolVersionDijkstra: data.NewConstr(0, data.NewInteger(amount)),
			},
		},
		{
			name: "explicit deregistration",
			certificate: &common.DeregistrationCertificate{
				StakeCredential: credential,
				Amount:          amount.Int64(),
			},
			want: map[uint]data.PlutusData{
				common.ProtocolVersionConway:   data.NewConstr(1),
				common.ProtocolVersionPlomin:   data.NewConstr(0, data.NewInteger(amount)),
				common.ProtocolVersionDijkstra: data.NewConstr(0, data.NewInteger(amount)),
			},
		},
		{
			name: "legacy registration",
			certificate: &common.StakeRegistrationCertificate{
				StakeCredential: credential,
			},
			want: map[uint]data.PlutusData{
				common.ProtocolVersionConway:   data.NewConstr(1),
				common.ProtocolVersionPlomin:   data.NewConstr(1),
				common.ProtocolVersionDijkstra: data.NewConstr(1),
			},
		},
		{
			name: "legacy deregistration",
			certificate: &common.StakeDeregistrationCertificate{
				StakeCredential: credential,
			},
			want: map[uint]data.PlutusData{
				common.ProtocolVersionConway:   data.NewConstr(1),
				common.ProtocolVersionPlomin:   data.NewConstr(1),
				common.ProtocolVersionDijkstra: data.NewConstr(1),
			},
		},
	}
	versions := []struct {
		name  string
		major uint
	}{
		{name: "PV9", major: common.ProtocolVersionConway},
		{name: "PV10", major: common.ProtocolVersionPlomin},
		{name: "Dijkstra", major: common.ProtocolVersionDijkstra},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tx := mockledger.NewTransactionBuilder().WithCertificates(tc.certificate)
			for _, version := range versions {
				t.Run(version.name, func(t *testing.T) {
					info, err := script.NewTxInfoV3FromTransaction(
						nil,
						tx,
						nil,
						version.major,
					)
					require.NoError(t, err)
					certificates := info.ToPlutusData().(*data.Constr).Fields[5].(*data.List)
					translated := certificates.Items[0].(*data.Constr).Fields[1]
					require.Equal(t, tc.want[version.major], translated)
				})
			}
		})
	}
}
