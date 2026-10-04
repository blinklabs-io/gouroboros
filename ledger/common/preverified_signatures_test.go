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
	"crypto/ed25519"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

type witnessKind string

const (
	vkeyWitness      witnessKind = "vkey"
	bootstrapWitness witnessKind = "bootstrap"
)

// newSignedConwayTx returns a transaction whose single witness of the given
// kind carries a valid signature over the transaction hash.
func newSignedConwayTx(
	t *testing.T,
	kind witnessKind,
	fee uint64,
	isValid bool,
) *conway.ConwayTransaction {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	tx := &conway.ConwayTransaction{TxIsValid: isValid}
	tx.Body.TxFee = fee
	// A constructed body has no stored CBOR, so without it every transaction
	// hashes the empty string.
	bodyCbor, err := cbor.Encode(&tx.Body)
	require.NoError(t, err)
	tx.Body.SetCbor(bodyCbor)
	hash := tx.Hash()
	sig := ed25519.Sign(priv, hash[:])
	switch kind {
	case vkeyWitness:
		tx.WitnessSet.VkeyWitnesses = cbor.NewSetType(
			[]common.VkeyWitness{{Vkey: pub, Signature: sig}},
			false,
		)
	case bootstrapWitness:
		tx.WitnessSet.BootstrapWitnesses = cbor.NewSetType(
			[]common.BootstrapWitness{{
				PublicKey:  pub,
				Signature:  sig,
				ChainCode:  make([]byte, 32),
				Attributes: []byte{0xa0},
			}},
			false,
		)
	}
	return tx
}

// corruptSignature invalidates the transaction's only witness signature.
func corruptSignature(tx *conway.ConwayTransaction, kind witnessKind) {
	switch kind {
	case vkeyWitness:
		tx.WitnessSet.VkeyWitnesses.Items()[0].Signature[0] ^= 0xff
	case bootstrapWitness:
		tx.WitnessSet.BootstrapWitnesses.Items()[0].Signature[0] ^= 0xff
	}
}

func verifySignatureRule(
	tx common.Transaction,
	signatures *common.PreverifiedSignatures,
) error {
	return common.VerifyTransactionWithSignatures(
		tx,
		0,
		mockledger.NewMockLedgerStateWithUtxos(nil),
		nil,
		[]common.UtxoValidationRuleFunc{common.UtxoValidateSignatures},
		signatures,
	)
}

// The kinds and validity flags cover vkey and bootstrap witnesses, and a
// phase-2-invalid transaction, whose witnesses must still be verified.
var preverifiedCases = []struct {
	name    string
	kind    witnessKind
	isValid bool
}{
	{"vkey", vkeyWitness, true},
	{"bootstrap", bootstrapWitness, true},
	{"vkey_phase2_invalid", vkeyWitness, false},
	{"bootstrap_phase2_invalid", bootstrapWitness, false},
}

// Changing a signature after preverification must force inline verification.
func TestPreverifiedSignaturesRejectChangedWitnesses(t *testing.T) {
	t.Parallel()
	for _, tc := range preverifiedCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tx := newSignedConwayTx(t, tc.kind, 1, tc.isValid)
			signatures := common.PreverifySignatures(tx)
			corruptSignature(tx, tc.kind)

			require.Error(t, verifySignatureRule(tx, signatures))
			require.Error(
				t,
				verifySignatureRule(tx, nil),
				"control: inline verification must see the corrupt signature",
			)
		})
	}
}

// Repairing a signature invalidates the cached failure as well.
func TestPreverifiedSignatureFailureIgnoredAfterRepair(t *testing.T) {
	t.Parallel()
	for _, tc := range preverifiedCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tx := newSignedConwayTx(t, tc.kind, 1, tc.isValid)
			corruptSignature(tx, tc.kind)
			inlineErr := verifySignatureRule(tx, nil)
			require.Error(t, inlineErr)

			signatures := common.PreverifySignatures(tx)
			corruptSignature(tx, tc.kind) // restore the valid signature
			require.NoError(t, verifySignatureRule(tx, signatures))
		})
	}
}

// A result computed for one transaction must not vouch for another.
func TestPreverifiedSignaturesIgnoredForOtherTransaction(t *testing.T) {
	t.Parallel()
	for _, tc := range preverifiedCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			other := newSignedConwayTx(t, tc.kind, 1, tc.isValid)
			signatures := common.PreverifySignatures(other)

			tx := newSignedConwayTx(t, tc.kind, 2, tc.isValid)
			corruptSignature(tx, tc.kind)

			require.Error(t, verifySignatureRule(tx, signatures))
		})
	}
}
