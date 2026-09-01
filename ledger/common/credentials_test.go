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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialUnmarshalCBORValidTypes(t *testing.T) {
	for _, credentialType := range []uint{
		common.CredentialTypeAddrKeyHash,
		common.CredentialTypeScriptHash,
	} {
		t.Run(fmt.Sprintf("type_%d", credentialType), func(t *testing.T) {
			encoded, err := cbor.Encode([]any{
				credentialType,
				make([]byte, common.Blake2b224Size),
			})
			require.NoError(t, err)

			var credential common.Credential
			_, err = cbor.Decode(encoded, &credential)
			require.NoError(t, err)
			assert.Equal(t, credentialType, credential.CredType)
		})
	}
}

func TestCredentialUnmarshalCBORRejectsInvalidTypes(t *testing.T) {
	for _, credentialType := range []uint{2, 255} {
		t.Run(fmt.Sprintf("type_%d", credentialType), func(t *testing.T) {
			encoded, err := cbor.Encode([]any{
				credentialType,
				make([]byte, common.Blake2b224Size),
			})
			require.NoError(t, err)

			var credential common.Credential
			_, err = cbor.Decode(encoded, &credential)
			require.ErrorContains(t, err, "invalid credential type")
		})
	}
}

func TestCredentialUnmarshalCBORRejectsNullAndUndefined(t *testing.T) {
	for _, cborData := range [][]byte{{0xf6}, {0xf7}} {
		var credential common.Credential
		require.Error(t, credential.UnmarshalCBOR(cborData))
	}
}

func TestCredentialFromJson(t *testing.T) {
	testDefs := []struct {
		json               string
		expectedCredential common.Credential
	}{
		{
			json: `"keyHash-95936e06ca168da5788019e78625166a2ea47bea1f2537ffd88ab426"`,
			expectedCredential: common.Credential{
				CredType: common.CredentialTypeAddrKeyHash,
				Credential: common.Blake2b224(
					func() []byte {
						foo, _ := hex.DecodeString(`95936e06ca168da5788019e78625166a2ea47bea1f2537ffd88ab426`)
						return foo
					}(),
				),
			},
		},
		{
			json: `"scriptHash-b9fa485ab33acd079d3553f42dac2ce5e12f1e1120fd1158d506e50b"`,
			expectedCredential: common.Credential{
				CredType: common.CredentialTypeScriptHash,
				Credential: common.Blake2b224(
					func() []byte {
						foo, _ := hex.DecodeString(`b9fa485ab33acd079d3553f42dac2ce5e12f1e1120fd1158d506e50b`)
						return foo
					}(),
				),
			},
		},
	}
	for _, testDef := range testDefs {
		var tmpCred common.Credential
		if err := json.Unmarshal([]byte(testDef.json), &tmpCred); err != nil {
			t.Errorf("unexpected JSON unmarshal failure: %s", err)
			continue
		}
		if !reflect.DeepEqual(tmpCred, testDef.expectedCredential) {
			t.Errorf("decoded credential does not match expected value\n     got: %#v\n  wanted: %#v", tmpCred, testDef.expectedCredential)
		}
	}
}
