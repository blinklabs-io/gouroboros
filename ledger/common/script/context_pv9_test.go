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

package script

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

func TestCertificateToPlutusDataUsesProtocolVersion(t *testing.T) {
	credential := common.Credential{Credential: common.Blake2b224{1}}
	cases := []struct {
		name        string
		certificate common.Certificate
		constructor *big.Int
	}{
		{
			name: "registration",
			certificate: &common.RegistrationCertificate{
				StakeCredential: credential,
				Amount:          2_000_000,
			},
			constructor: big.NewInt(0),
		},
		{
			name: "deregistration",
			certificate: &common.DeregistrationCertificate{
				StakeCredential: credential,
				Amount:          2_000_000,
			},
			constructor: big.NewInt(1),
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			pv9 := certificateToPlutusData(tt.certificate, common.ProtocolVersionConway)
			pv10 := certificateToPlutusData(tt.certificate, common.ProtocolVersionPlomin)
			pv9Fields := pv9.(*data.Constr).Fields
			pv10Fields := pv10.(*data.Constr).Fields
			require.Equal(t, tt.constructor, pv9.(*data.Constr).Tag)
			require.Equal(t, data.NewConstr(1), pv9Fields[1])
			require.Equal(
				t,
				data.NewConstr(0, data.NewInteger(big.NewInt(2_000_000))),
				pv10Fields[1],
			)
			pv9Bytes, err := data.Encode(pv9)
			require.NoError(t, err)
			pv10Bytes, err := data.Encode(pv10)
			require.NoError(t, err)
			require.NotEqual(t, pv9Bytes, pv10Bytes)
		})
	}
}

func TestTxInfoV3CertificateTranslationUsesProtocolVersion(t *testing.T) {
	certificate := &common.RegistrationCertificate{
		StakeCredential: common.Credential{Credential: common.Blake2b224{1}},
		Amount:          2_000_000,
	}
	for _, tc := range []struct {
		major uint
		want  data.PlutusData
	}{
		{major: common.ProtocolVersionConway, want: data.NewConstr(1)},
		{
			major: common.ProtocolVersionPlomin,
			want:  data.NewConstr(0, data.NewInteger(big.NewInt(2_000_000))),
		},
	} {
		info := TxInfoV3{
			Certificates:         []common.Certificate{certificate},
			ProtocolVersionMajor: tc.major,
			Mint:                 common.MultiAsset[common.MultiAssetTypeMint]{},
		}
		fields := info.ToPlutusData().(*data.Constr).Fields
		translated := fields[5].(*data.List).Items[0].(*data.Constr).Fields[1]
		require.Equal(t, tc.want, translated)
	}
}

func TestScriptPurposeCertificateTranslationUsesProtocolVersion(t *testing.T) {
	certificate := &common.RegistrationCertificate{
		StakeCredential: common.Credential{Credential: common.Blake2b224{1}},
		Amount:          2_000_000,
	}
	purpose, err := BuildScriptPurpose(
		common.RedeemerKey{Tag: common.RedeemerTagCert},
		nil,
		nil,
		common.MultiAsset[common.MultiAssetTypeMint]{},
		[]common.Certificate{certificate},
		nil,
		nil,
		nil,
		nil,
		common.ProtocolVersionConway,
	)
	require.NoError(t, err)
	purposeData := purpose.ToPlutusData().(*data.Constr)
	certificateData := purposeData.Fields[1].(*data.Constr)
	require.Equal(t, data.NewConstr(1), certificateData.Fields[1])
}
