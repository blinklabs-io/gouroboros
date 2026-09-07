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

	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

func TestHeaderHashConcurrentFirstFill(t *testing.T) {
	cborData := make([]byte, 1<<20)
	shelleyHeader := &shelley.ShelleyBlockHeader{}
	shelleyHeader.SetCbor(cborData)
	babbageHeader := &babbage.BabbageBlockHeader{}
	babbageHeader.SetCbor(cborData)
	byronHeader := &byron.ByronMainBlockHeader{}
	byronHeader.SetCbor(cborData)
	ebbHeader := &byron.ByronEpochBoundaryBlockHeader{}
	ebbHeader.SetCbor(cborData)
	tests := []struct {
		name string
		hash func() common.Blake2b256
	}{
		{name: "shelley", hash: shelleyHeader.Hash},
		{name: "babbage", hash: babbageHeader.Hash},
		{name: "byron", hash: byronHeader.Hash},
		{name: "byron ebb", hash: ebbHeader.Hash},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var start sync.WaitGroup
			start.Add(1)
			var workers sync.WaitGroup
			for range 32 {
				workers.Add(1)
				go func() {
					defer workers.Done()
					start.Wait()
					test.hash()
				}()
			}
			start.Done()
			workers.Wait()
		})
	}
}
