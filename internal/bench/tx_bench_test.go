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
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
)

// benchSink prevents compiler dead-code elimination in benchmarks.
var benchSink interface{}

// BenchmarkTxDecode benchmarks transaction CBOR decoding by era.
func BenchmarkTxDecode(b *testing.B) {
	for _, era := range PostByronEraNames() {
		fixture := MustLoadBlockFixture(era, "default")
		txs := fixture.Block.Transactions()
		if len(txs) == 0 {
			continue
		}
		txCbor := txs[0].Cbor()
		if len(txCbor) == 0 {
			continue
		}

		txType, err := TxTypeFromEra(era)
		if err != nil {
			b.Fatalf("TxTypeFromEra failed for %s: %v", era, err)
		}

		b.Run("Era_"+era, func(b *testing.B) {
			// Pre-validate that decoding succeeds before measuring
			tx, err := ledger.NewTransactionFromCbor(txType, txCbor)
			if err != nil {
				b.Fatalf(
					"NewTransactionFromCbor failed for %s: %v",
					era,
					err,
				)
			}
			benchSink = tx

			b.SetBytes(int64(len(txCbor)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSink, _ = ledger.NewTransactionFromCbor(txType, txCbor)
			}
		})
	}
}

// BenchmarkTxHash benchmarks transaction hash calculation.
func BenchmarkTxHash(b *testing.B) {
	for _, era := range PostByronEraNames() {
		fixture := MustLoadBlockFixture(era, "default")
		txs := fixture.Block.Transactions()
		if len(txs) == 0 {
			continue
		}
		tx := txs[0]

		b.Run("Era_"+era, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSink = tx.Hash()
			}
		})
	}
}

// BenchmarkTxInputs benchmarks transaction input iteration.
func BenchmarkTxInputs(b *testing.B) {
	for _, era := range PostByronEraNames() {
		fixture := MustLoadBlockFixture(era, "default")
		txs := fixture.Block.Transactions()
		if len(txs) == 0 {
			continue
		}
		tx := txs[0]

		b.Run("Era_"+era, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSink = tx.Inputs()
			}
		})
	}
}

// BenchmarkTxOutputs benchmarks transaction output iteration.
func BenchmarkTxOutputs(b *testing.B) {
	for _, era := range PostByronEraNames() {
		fixture := MustLoadBlockFixture(era, "default")
		txs := fixture.Block.Transactions()
		if len(txs) == 0 {
			continue
		}
		tx := txs[0]

		b.Run("Era_"+era, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSink = tx.Outputs()
			}
		})
	}
}

// BenchmarkTxFee benchmarks transaction fee retrieval.
func BenchmarkTxFee(b *testing.B) {
	for _, era := range PostByronEraNames() {
		fixture := MustLoadBlockFixture(era, "default")
		txs := fixture.Block.Transactions()
		if len(txs) == 0 {
			continue
		}
		tx := txs[0]

		b.Run("Era_"+era, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSink = tx.Fee()
			}
		})
	}
}

// BenchmarkTxMetadata benchmarks transaction metadata retrieval.
func BenchmarkTxMetadata(b *testing.B) {
	for _, era := range PostByronEraNames() {
		fixture := MustLoadBlockFixture(era, "default")
		txs := fixture.Block.Transactions()
		if len(txs) == 0 {
			continue
		}
		tx := txs[0]

		b.Run("Era_"+era, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSink = tx.Metadata()
			}
		})
	}
}

// BenchmarkTxCbor benchmarks getting transaction CBOR bytes.
func BenchmarkTxCbor(b *testing.B) {
	for _, era := range PostByronEraNames() {
		fixture := MustLoadBlockFixture(era, "default")
		txs := fixture.Block.Transactions()
		if len(txs) == 0 {
			continue
		}
		tx := txs[0]

		b.Run("Era_"+era, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSink = tx.Cbor()
			}
		})
	}
}

// BenchmarkTxUtxorpc benchmarks transaction Utxorpc conversion.
func BenchmarkTxUtxorpc(b *testing.B) {
	for _, era := range PostByronEraNames() {
		fixture := MustLoadBlockFixture(era, "default")
		txs := fixture.Block.Transactions()
		if len(txs) == 0 {
			continue
		}
		tx := txs[0]

		b.Run("Era_"+era, func(b *testing.B) {
			// Pre-validate that Utxorpc conversion succeeds
			result, err := tx.Utxorpc()
			if err != nil {
				b.Fatalf("Utxorpc failed for %s: %v", era, err)
			}
			benchSink = result

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSink, _ = tx.Utxorpc()
			}
		})
	}
}

// BenchmarkTxProducedUtxos benchmarks produced UTXO enumeration.
func BenchmarkTxProducedUtxos(b *testing.B) {
	for _, era := range PostByronEraNames() {
		fixture := MustLoadBlockFixture(era, "default")
		txs := fixture.Block.Transactions()
		if len(txs) == 0 {
			continue
		}
		tx := txs[0]

		b.Run("Era_"+era, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSink = tx.Produced()
			}
		})
	}
}

// BenchmarkTxConsumedUtxos benchmarks consumed UTXO enumeration.
func BenchmarkTxConsumedUtxos(b *testing.B) {
	for _, era := range PostByronEraNames() {
		fixture := MustLoadBlockFixture(era, "default")
		txs := fixture.Block.Transactions()
		if len(txs) == 0 {
			continue
		}
		tx := txs[0]

		b.Run("Era_"+era, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSink = tx.Consumed()
			}
		})
	}
}

// BenchmarkTxBlockIteration benchmarks iterating through all transactions in a
// block.
func BenchmarkTxBlockIteration(b *testing.B) {
	for _, era := range PostByronEraNames() {
		fixture := MustLoadBlockFixture(era, "default")

		b.Run("Era_"+era, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				txs := fixture.Block.Transactions()
				for _, tx := range txs {
					benchSink = tx.Hash()
					benchSink = tx.Fee()
					benchSink = tx.Inputs()
					benchSink = tx.Outputs()
				}
			}
		})
	}
}
