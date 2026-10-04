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

package common

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type bindingTestWitnesses struct {
	TransactionWitnessSet
	vkeys []VkeyWitness
}

func (w bindingTestWitnesses) Vkey() []VkeyWitness { return w.vkeys }

func (w bindingTestWitnesses) Bootstrap() []BootstrapWitness { return nil }

// bindingTestTx has no inputs, so only the signature checks can fail.
type bindingTestTx struct {
	Transaction
	witnesses TransactionWitnessSet
}

func (t bindingTestTx) Hash() Blake2b256 { return Blake2b256{1} }

func (t bindingTestTx) Witnesses() TransactionWitnessSet { return t.witnesses }

func (t bindingTestTx) Inputs() []TransactionInput { return nil }

func newBindingTestTx() bindingTestTx {
	return bindingTestTx{
		witnesses: bindingTestWitnesses{
			vkeys: []VkeyWitness{{
				Vkey:      make([]byte, 32),
				Signature: make([]byte, 64),
			}},
		},
	}
}

// A result whose binding matches the transaction is trusted instead of
// verifying again. The witness is not validly signed, so only reuse can pass.
func TestUtxoValidateSignaturesReusesMatchingResult(t *testing.T) {
	t.Parallel()
	tx := newBindingTestTx()
	require.Error(t, ValidateVKeyWitnesses(tx), "control: witness is invalid")

	ls := &cachedLedgerState{
		signatures: &PreverifiedSignatures{binding: signatureBinding(tx)},
	}
	require.NoError(t, UtxoValidateSignatures(tx, 0, ls, nil))
}

// A recorded failure is returned unchanged rather than recomputed.
func TestUtxoValidateSignaturesReturnsRecordedFailure(t *testing.T) {
	t.Parallel()
	tx := newBindingTestTx()
	recorded := errors.New("recorded failure")

	ls := &cachedLedgerState{
		signatures: &PreverifiedSignatures{
			binding: signatureBinding(tx),
			err:     recorded,
		},
	}
	require.Same(t, recorded, UtxoValidateSignatures(tx, 0, ls, nil))
}
