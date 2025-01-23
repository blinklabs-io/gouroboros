// Copyright 2023 Blink Labs Software
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
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
)

func TestDetermineTransactionType(t *testing.T) {
	testDefs := []struct {
		txCborHex      string
		expectedTxType uint
	}{
		{
			txCborHex:      "84a500d9010281825820279184037d249e397d97293738370756da559718fcdefae9924834840046b37b01018282583900923d4b64e1d730a4baf3e6dc433a9686983940f458363f37aad7a1a9568b72f85522e4a17d44a45cd021b9741b55d7cbc635c911625b015e1a00a9867082583900923d4b64e1d730a4baf3e6dc433a9686983940f458363f37aad7a1a9568b72f85522e4a17d44a45cd021b9741b55d7cbc635c911625b015e1b00000001267d7b04021a0002938d031a04e304e70800a100d9010281825820b829480e5d5827d2e1bd7c89176a5ca125c30812e54be7dbdf5c47c835a17f3d5840b13a76e7f2b19cde216fcad55ceeeb489ebab3dcf63ef1539ac4f535dece00411ee55c9b8188ef04b4aa3c72586e4a0ec9b89949367d7270fdddad3b18731403f5f6",
			expectedTxType: 6,
		},
	}
	for _, testDef := range testDefs {
		txCbor, err := hex.DecodeString(testDef.txCborHex)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		tmpTxType, err := ledger.DetermineTransactionType(txCbor)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if tmpTxType != testDef.expectedTxType {
			t.Fatalf("did not get expected TX type: got %d, wanted %d", tmpTxType, testDef.expectedTxType)
		}
	}
}
