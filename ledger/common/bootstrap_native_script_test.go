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

package common_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"
)

func bootstrapTestWitness() common.BootstrapWitness {
	return common.BootstrapWitness{
		PublicKey:  bytes.Repeat([]byte{0x41}, 32),
		Signature:  bytes.Repeat([]byte{0x42}, 64),
		ChainCode:  bytes.Repeat([]byte{0x43}, 32),
		Attributes: []byte{0xa0},
	}
}

// bootstrapTestAddrRoot spells out the address root independently of the
// production helper so the expected value is not read back from it.
func bootstrapTestAddrRoot(w common.BootstrapWitness) common.Blake2b224 {
	payload := []byte{0x83, 0x00, 0x82, 0x00, 0x58, 0x40}
	payload = append(payload, w.PublicKey...)
	payload = append(payload, w.ChainCode...)
	payload = append(payload, w.Attributes...)
	sum := sha3.Sum256(payload)
	return common.Blake2b224Hash(sum[:])
}

func nativeScriptRequiringHash(
	t *testing.T,
	hash common.Blake2b224,
) common.NativeScript {
	t.Helper()
	scriptCbor, err := cbor.Encode(common.NativeScriptPubkey{
		Type: 0,
		Hash: hash.Bytes(),
	})
	require.NoError(t, err)
	var ns common.NativeScript
	require.NoError(t, ns.UnmarshalCBOR(scriptCbor))
	return ns
}

func TestBootstrapWitnessKeyHashIsAddressRoot(t *testing.T) {
	t.Parallel()
	w := bootstrapTestWitness()
	got, err := w.KeyHash()
	require.NoError(t, err)
	require.Equal(t, bootstrapTestAddrRoot(w), got)
	require.NotEqual(t, common.Blake2b224Hash(w.PublicKey), got)
}

func TestBootstrapWitnessKeyHashFailsClosed(t *testing.T) {
	t.Parallel()
	w := bootstrapTestWitness()
	w.PublicKey = w.PublicKey[:31]
	_, err := w.KeyHash()
	require.Error(t, err)
}

func TestFirstInvalidNativeScriptInExcludesBootstrapWitnesses(t *testing.T) {
	t.Parallel()
	w := bootstrapTestWitness()
	bootstrapOnly := mockledger.NewTransactionBuilder().WithWitnesses(
		mockledger.NewMockTransactionWitnessSet().
			WithBootstrapWitnesses(w),
	)

	t.Run("public key hash is not satisfied", func(t *testing.T) {
		t.Parallel()
		ns := nativeScriptRequiringHash(t, common.Blake2b224Hash(w.PublicKey))
		_, invalid := common.FirstInvalidNativeScriptIn(
			bootstrapOnly, 0, []common.NativeScript{ns},
		)
		require.True(t, invalid)
	})

	t.Run("address root is not satisfied", func(t *testing.T) {
		t.Parallel()
		ns := nativeScriptRequiringHash(t, bootstrapTestAddrRoot(w))
		_, invalid := common.FirstInvalidNativeScriptIn(
			bootstrapOnly, 0, []common.NativeScript{ns},
		)
		require.True(t, invalid)
	})

	t.Run("vkey witness still satisfies public key hash", func(t *testing.T) {
		t.Parallel()
		tx := mockledger.NewTransactionBuilder().WithWitnesses(
			mockledger.NewMockTransactionWitnessSet().
				WithVkeyWitnesses(common.VkeyWitness{Vkey: w.PublicKey}),
		)
		ns := nativeScriptRequiringHash(t, common.Blake2b224Hash(w.PublicKey))
		_, invalid := common.FirstInvalidNativeScriptIn(
			tx, 0, []common.NativeScript{ns},
		)
		require.False(t, invalid)
	})
}

func TestNativeScriptRulesExcludeBootstrapWitnesses(t *testing.T) {
	t.Parallel()
	w := bootstrapTestWitness()
	ledgerState := mockledger.NewLedgerStateBuilder().Build()

	tests := []struct {
		name string
		rule common.UtxoValidationRuleFunc
	}{
		{name: "Shelley", rule: shelley.UtxoValidateNativeScripts},
		{name: "Allegra", rule: allegra.UtxoValidateNativeScripts},
		{name: "Mary", rule: mary.UtxoValidateNativeScripts},
		{name: "Alonzo", rule: alonzo.UtxoValidateNativeScripts},
		{name: "Babbage", rule: babbage.UtxoValidateNativeScripts},
		{name: "Conway", rule: conway.UtxoValidateNativeScripts},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			build := func(
				native common.NativeScript,
				witnesses *mockledger.MockTransactionWitnessSet,
			) *mockledger.MockTransaction {
				credential := common.Credential{
					CredType:   common.CredentialTypeScriptHash,
					Credential: common.Blake2b224(native.Hash()),
				}
				return mockledger.NewTransactionBuilder().
					WithCertificates(&common.DeregistrationCertificate{
						StakeCredential: credential,
					}).
					WithWitnesses(witnesses.WithNativeScripts(native))
			}

			t.Run("bootstrap witness", func(t *testing.T) {
				native := nativeScriptRequiringHash(
					t,
					bootstrapTestAddrRoot(w),
				)
				tx := build(
					native,
					mockledger.NewMockTransactionWitnessSet().
						WithBootstrapWitnesses(w),
				)
				require.Error(t, test.rule(tx, 0, ledgerState, nil))
			})

			t.Run("vkey witness", func(t *testing.T) {
				native := nativeScriptRequiringHash(
					t,
					common.Blake2b224Hash(w.PublicKey),
				)
				tx := build(
					native,
					mockledger.NewMockTransactionWitnessSet().
						WithVkeyWitnesses(common.VkeyWitness{
							Vkey: w.PublicKey,
						}),
				)
				require.NoError(t, test.rule(tx, 0, ledgerState, nil))
			})
		})
	}

	t.Run("Dijkstra", func(t *testing.T) {
		t.Parallel()
		build := func(
			native common.NativeScript,
			bootstrap []common.BootstrapWitness,
			vkeys []common.VkeyWitness,
		) *dijkstra.DijkstraTransaction {
			credential := common.Credential{
				CredType:   common.CredentialTypeScriptHash,
				Credential: common.Blake2b224(native.Hash()),
			}
			return &dijkstra.DijkstraTransaction{
				Body: dijkstra.DijkstraTransactionBody{
					TxGuards: &dijkstra.DijkstraGuards{
						Credentials: []common.Credential{credential},
					},
				},
				WitnessSet: dijkstra.DijkstraTransactionWitnessSet{
					VkeyWitnesses: cbor.NewSetType(vkeys, false),
					WsNativeScripts: cbor.NewSetType(
						[]common.NativeScript{native},
						false,
					),
					BootstrapWitnesses: cbor.NewSetType(bootstrap, false),
				},
				TxIsValid: true,
			}
		}

		t.Run("bootstrap witness", func(t *testing.T) {
			native := nativeScriptRequiringHash(t, bootstrapTestAddrRoot(w))
			tx := build(native, []common.BootstrapWitness{w}, nil)
			require.Error(
				t,
				dijkstra.UtxoValidateNativeScripts(tx, 0, ledgerState, nil),
			)
		})

		t.Run("vkey witness", func(t *testing.T) {
			native := nativeScriptRequiringHash(
				t,
				common.Blake2b224Hash(w.PublicKey),
			)
			tx := build(
				native,
				nil,
				[]common.VkeyWitness{{Vkey: w.PublicKey}},
			)
			require.NoError(
				t,
				dijkstra.UtxoValidateNativeScripts(tx, 0, ledgerState, nil),
			)
		})
	})
}
