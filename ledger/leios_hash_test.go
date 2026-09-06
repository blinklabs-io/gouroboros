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

package ledger_test

import (
	"sync"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

const testTransactionCbor = "\x83\xa0\xa0\xa0"

func TestLeiosHashFreshFirstCallRace(t *testing.T) {
	for _, test := range leiosHashTests() {
		t.Run(test.name, func(t *testing.T) {
			tx := test.new()
			const workers = 32
			results := make(chan common.Blake2b256, workers)
			var wg sync.WaitGroup
			for range workers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					results <- tx.LeiosHash()
				}()
			}
			wg.Wait()
			close(results)
			want := common.Blake2b256Hash([]byte(testTransactionCbor))
			for got := range results {
				require.Equal(t, want, got)
			}
		})
	}
}

func TestLeiosHashValueAndPointerReceivers(t *testing.T) {
	for _, test := range leiosHashTests() {
		t.Run(test.name, func(t *testing.T) {
			tx := test.new()
			require.Equal(t, test.valueHash(tx), tx.LeiosHash())
		})
	}
}

type leiosHashTest struct {
	name      string
	new       func() common.Transaction
	valueHash func(common.Transaction) common.Blake2b256
}

func leiosHashTests() []leiosHashTest {
	data := []byte(testTransactionCbor)
	return []leiosHashTest{
		{
			"byron",
			func() common.Transaction { tx := &byron.ByronTransaction{}; tx.SetCbor(data); return tx },
			func(tx common.Transaction) common.Blake2b256 {
				value := *tx.(*byron.ByronTransaction)
				return value.LeiosHash()
			},
		},
		{
			"shelley",
			func() common.Transaction { tx := &shelley.ShelleyTransaction{}; tx.SetCbor(data); return tx },
			func(tx common.Transaction) common.Blake2b256 {
				value := *tx.(*shelley.ShelleyTransaction)
				return value.LeiosHash()
			},
		},
		{
			"allegra",
			func() common.Transaction { tx := &allegra.AllegraTransaction{}; tx.SetCbor(data); return tx },
			func(tx common.Transaction) common.Blake2b256 {
				value := *tx.(*allegra.AllegraTransaction)
				return value.LeiosHash()
			},
		},
		{
			"mary",
			func() common.Transaction { tx := &mary.MaryTransaction{}; tx.SetCbor(data); return tx },
			func(tx common.Transaction) common.Blake2b256 {
				value := *tx.(*mary.MaryTransaction)
				return value.LeiosHash()
			},
		},
		{
			"alonzo",
			func() common.Transaction { tx := &alonzo.AlonzoTransaction{}; tx.SetCbor(data); return tx },
			func(tx common.Transaction) common.Blake2b256 {
				value := *tx.(*alonzo.AlonzoTransaction)
				return value.LeiosHash()
			},
		},
		{
			"babbage",
			func() common.Transaction { tx := &babbage.BabbageTransaction{}; tx.SetCbor(data); return tx },
			func(tx common.Transaction) common.Blake2b256 {
				value := *tx.(*babbage.BabbageTransaction)
				return value.LeiosHash()
			},
		},
		{
			"conway",
			func() common.Transaction { tx := &conway.ConwayTransaction{}; tx.SetCbor(data); return tx },
			func(tx common.Transaction) common.Blake2b256 {
				value := *tx.(*conway.ConwayTransaction)
				return value.LeiosHash()
			},
		},
		{
			"dijkstra",
			func() common.Transaction { tx := &dijkstra.DijkstraTransaction{}; tx.SetCbor(data); return tx },
			func(tx common.Transaction) common.Blake2b256 {
				value := *tx.(*dijkstra.DijkstraTransaction)
				return value.LeiosHash()
			},
		},
	}
}
