package ledger

import (
	"testing"
)

func TestBabbageBlockTransactions(t *testing.T) {
	b := &BabbageBlock{}

	t.Run("empty", func(t *testing.T) {
		if lenTxs := len(b.Transactions()); lenTxs != 0 {
			t.Fatalf("expected number of transactions is 0 but it was %d", lenTxs)
		}
	})

	txsCount := 7
	b.TransactionBodies = make([]BabbageTransactionBody, txsCount)
	b.TransactionWitnessSets = make([]BabbageTransactionWitnessSet, txsCount)

	for i := 0; i < txsCount; i++ {
		b.TransactionBodies[i] = BabbageTransactionBody{
			TotalCollateral: 1 << i,
		}
		b.TransactionWitnessSets[i] = BabbageTransactionWitnessSet{
			AlonzoTransactionWitnessSet: AlonzoTransactionWitnessSet{
				ShelleyTransactionWitnessSet: ShelleyTransactionWitnessSet{
					VkeyWitnesses: []interface{}{
						append(make([]byte, 95), 1<<i),
					},
				},
			},
		}
	}

	t.Run("no invalid", func(t *testing.T) {
		txs := b.Transactions()

		if lenTxs := len(txs); lenTxs != txsCount {
			t.Fatalf("expected number of transactions is %d but it was %d", txsCount, lenTxs)
		}

		for i, tx := range txs {
			if btx := tx.(*BabbageTransaction); !btx.IsValid() {
				t.Fatalf("expected transaction number %d IsValid is %v but is was %v", i, true, btx.IsValid())
			}
		}
	})

	t.Run("with invalid", func(t *testing.T) {
		b.InvalidTransactions = []uint{2, 4, 5}
		txs := b.Transactions()

		if lenTxs := len(txs); lenTxs != txsCount {
			t.Fatalf("expected number of transactions is %d but it was %d", txsCount, lenTxs)
		}

		for i, tx := range txs {
			expected := i != 2 && i != 4 && i != 5
			if btx := tx.(*BabbageTransaction); btx.IsValid() != expected {
				t.Fatalf("expected transaction number %d IsValid is %v but is was %v", i, expected, btx.IsValid())
			}
		}
	})
}
