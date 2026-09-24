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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/builtin"
	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
	"github.com/stretchr/testify/require"
)

func encodeCertificatesAssertScript(t *testing.T, want data.PlutusData) common.PlutusV3Script {
	t.Helper()
	applyBuiltin := func(fn builtin.DefaultFunction, args ...syn.Term[syn.DeBruijn]) syn.Term[syn.DeBruijn] {
		var term syn.Term[syn.DeBruijn] = &syn.Builtin{DefaultFunction: fn}
		forces := 0
		switch fn {
		case builtin.SndPair:
			forces = 2
		case builtin.HeadList, builtin.TailList, builtin.IfThenElse:
			forces = 1
		}
		for range forces {
			term = &syn.Force[syn.DeBruijn]{Term: term}
		}
		for _, arg := range args {
			term = &syn.Apply[syn.DeBruijn]{Function: term, Argument: arg}
		}
		return term
	}
	ctx := syn.Term[syn.DeBruijn](&syn.Var[syn.DeBruijn]{Name: 1})
	contextFields := applyBuiltin(builtin.SndPair, applyBuiltin(builtin.UnConstrData, ctx))
	txInfo := applyBuiltin(builtin.HeadList, contextFields)
	constructorFields := applyBuiltin(builtin.SndPair, applyBuiltin(builtin.UnConstrData, txInfo))
	certificatesData := applyBuiltin(builtin.HeadList, applyBuiltin(
		builtin.TailList, applyBuiltin(builtin.TailList, applyBuiltin(
			builtin.TailList, applyBuiltin(builtin.TailList, applyBuiltin(
				builtin.TailList, constructorFields,
			)),
		)),
	))
	certificates := applyBuiltin(builtin.UnListData, certificatesData)
	certificate := applyBuiltin(builtin.HeadList, certificates)
	condition := applyBuiltin(
		builtin.EqualsData,
		certificate,
		&syn.Constant{Con: &syn.Data{Inner: want}},
	)
	term := syn.Term[syn.DeBruijn](&syn.Force[syn.DeBruijn]{Term: applyBuiltin(
		builtin.IfThenElse,
		condition,
		&syn.Delay[syn.DeBruijn]{Term: &syn.Constant{Con: &syn.Unit{}}},
		&syn.Delay[syn.DeBruijn]{Term: &syn.Error{}},
	)})
	flat, err := syn.Encode(&syn.Program[syn.DeBruijn]{
		Version: lang.LanguageVersion{1, 1, 0},
		Term:    &syn.Lambda[syn.DeBruijn]{Body: term},
	})
	require.NoError(t, err)
	wrapper, err := cbor.Encode(flat)
	require.NoError(t, err)
	return common.PlutusV3Script(wrapper)
}

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

func TestConwayPhase2SeesCertificateAmountByProtocolVersion(t *testing.T) {
	credential := common.Credential{Credential: common.Blake2b224{1}}
	amount := big.NewInt(2_000_000)
	explicit := mockledger.NewTransactionBuilder().WithCertificates(
		&common.RegistrationCertificate{StakeCredential: credential, Amount: amount.Int64()},
	)
	legacy := mockledger.NewTransactionBuilder().WithCertificates(
		&common.StakeRegistrationCertificate{StakeCredential: credential},
	)
	credentialData := data.NewConstr(0, &data.ByteString{Inner: credential.Credential[:]})
	wantExplicit := data.NewConstr(
		0,
		credentialData,
		data.NewConstr(0, data.NewInteger(amount)),
	)
	wantAbsent := data.NewConstr(
		0,
		credentialData,
		data.NewConstr(1),
	)
	evalContext, err := cek.NewEvalContext(
		cek.LanguageVersionV3,
		cek.ProtoVersion{Major: 11},
		testdata.Epoch653PlutusV3CostModel,
	)
	require.NoError(t, err)
	for _, tc := range []struct {
		name     string
		tx       *mockledger.MockTransaction
		version  uint
		wantData data.PlutusData
	}{
		{name: "PV9 explicit amount is absent", tx: explicit, version: common.ProtocolVersionConway, wantData: wantAbsent},
		{name: "PV10 explicit amount is present", tx: explicit, version: common.ProtocolVersionPlomin, wantData: wantExplicit},
		{name: "Dijkstra explicit amount is present", tx: explicit, version: common.ProtocolVersionDijkstra, wantData: wantExplicit},
		{name: "PV10 legacy amount is absent", tx: legacy, version: common.ProtocolVersionPlomin, wantData: wantAbsent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info, err := script.NewTxInfoV3FromTransaction(nil, tc.tx, nil, tc.version)
			require.NoError(t, err)
			_, err = encodeCertificatesAssertScript(t, tc.wantData).Evaluate(
				data.NewConstr(0, info.ToPlutusData(), data.NewConstr(0), data.NewConstr(0)),
				common.ExUnits{},
				evalContext,
			)
			require.NoError(t, err)
		})
	}
}
