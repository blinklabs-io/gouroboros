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

package bench

import (
	"sync"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

var signatureRules = []common.UtxoValidationRuleFunc{
	common.UtxoValidateSignatures,
}

// preverifyBlock verifies every transaction's signatures concurrently, one
// goroutine per transaction.
func preverifyBlock(
	txs []common.Transaction,
) []*common.PreverifiedSignatures {
	results := make([]*common.PreverifiedSignatures, len(txs))
	var wg sync.WaitGroup
	for i, tx := range txs {
		wg.Go(func() {
			results[i] = common.PreverifySignatures(tx)
		})
	}
	wg.Wait()
	return results
}

func validateBlockSignatures(
	txs []common.Transaction,
	results []*common.PreverifiedSignatures,
) error {
	ls := BenchLedgerState()
	pp := BenchProtocolParams()
	for i, tx := range txs {
		var pre *common.PreverifiedSignatures
		if results != nil {
			pre = results[i]
		}
		if err := common.VerifyTransactionWithSignatures(
			tx, 0, ls, pp, signatureRules, pre,
		); err != nil {
			return err
		}
	}
	return nil
}

// TestSignaturePreverificationMatchesSerial validates every fixture block
// serially and through concurrent pre-verification; run under -race it shows
// the primitive is safe to call across a block's transactions.
func TestSignaturePreverificationMatchesSerial(t *testing.T) {
	t.Parallel()
	for _, era := range PostByronEraNames() {
		t.Run(era, func(t *testing.T) {
			t.Parallel()
			txs := MustLoadBlockFixture(era, "default").Block.Transactions()
			require.NoError(t, validateBlockSignatures(txs, nil))
			require.NoError(
				t,
				validateBlockSignatures(txs, preverifyBlock(txs)),
			)
		})
	}
}

// BenchmarkSignatureVerification compares the serial per-transaction
// signature rule with concurrent pre-verification followed by validation that
// reuses the results, over the transactions of one block.
func BenchmarkSignatureVerification(b *testing.B) {
	for _, era := range PostByronEraNames() {
		txs := MustLoadBlockFixture(era, "default").Block.Transactions()
		if len(txs) == 0 {
			continue
		}
		b.Run("Era_"+era+"/serial", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := validateBlockSignatures(txs, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("Era_"+era+"/preverified_parallel", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := validateBlockSignatures(
					txs,
					preverifyBlock(txs),
				); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
