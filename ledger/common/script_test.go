// Copyright 2025 Blink Labs Software
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
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

func TestScriptRefDecode(t *testing.T) {
	// 24_0(<<[3, h'480123456789abcdef']>>)
	testCbor, _ := hex.DecodeString("d8184c820349480123456789abcdef")
	scriptCbor, _ := hex.DecodeString("480123456789abcdef")
	expectedScript := common.PlutusV3Script(scriptCbor)
	var testScriptRef common.ScriptRef
	if _, err := cbor.Decode(testCbor, &testScriptRef); err != nil {
		t.Fatalf("unexpected error decoding script ref CBOR: %s", err)
	}
	if !reflect.DeepEqual(testScriptRef.Script, &expectedScript) {
		t.Fatalf(
			"did not get expected script\n     got: %#v\n  wanted: %#v",
			testScriptRef.Script,
			&expectedScript,
		)
	}
}
