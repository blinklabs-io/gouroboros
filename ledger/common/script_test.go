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

func TestScriptRefDecodeEncode(t *testing.T) {
	// 24_0(<<[3, h'480123456789abcdef']>>)
	testCborHex := "d8184c820349480123456789abcdef"
	testCbor, _ := hex.DecodeString(testCborHex)
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
	scriptRefCbor, err := cbor.Encode(testScriptRef)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if hex.EncodeToString(scriptRefCbor) != testCborHex {
		t.Fatalf("did not get expected CBOR\n     got: %x\n  wanted: %s", scriptRefCbor, testCborHex)
	}
}

func TestNativeScriptHash(t *testing.T) {
	testScriptBytes, _ := hex.DecodeString(
		"820181830301838200581c058a5ab0c66647dcce82d7244f80bfea41ba76c7c9ccaf86a41b00fe8200581c45cbc234959cb619ef54e36c16e7719318592e627cdf1a39bd3d64398200581c85fd53e110449649b709ef0fa93e86d99535bdce5db306ce0e7418fc",
	)
	expectedScriptHash := "1c0053ec18e2c0f7bd4d007fe14243ca220563f9c124381f75c43704"
	var testScript common.NativeScript
	if err := testScript.UnmarshalCBOR(testScriptBytes); err != nil {
		t.Fatalf("unexpected error decoding native script: %s", err)
	}
	tmpHash := testScript.Hash()
	if tmpHash.String() != expectedScriptHash {
		t.Errorf(
			"did not get expected script hash, got %s, wanted %s",
			tmpHash.String(),
			expectedScriptHash,
		)
	}
}

func TestPlutusV3ScriptHash(t *testing.T) {
	testScriptBytes, _ := hex.DecodeString(
		"587f01010032323232323225333002323232323253330073370e900118041baa0011323232533300a3370e900018059baa00513232533300f301100214a22c6eb8c03c004c030dd50028b18069807001180600098049baa00116300a300b0023009001300900230070013004375400229309b2b2b9a5573aaae7955cfaba157441",
	)
	testScript := common.PlutusV3Script(testScriptBytes)
	expectedScriptHash := "2909c3d0441e76cd6ae1fc09664bb209868902e191c2b8c30b82d331"
	tmpHash := testScript.Hash()
	if tmpHash.String() != expectedScriptHash {
		t.Errorf(
			"did not get expected script hash, got %s, wanted %s",
			tmpHash.String(),
			expectedScriptHash,
		)
	}
}
