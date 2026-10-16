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

package testdata

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
)

// PlutusScript returns a script that succeeds when given enough budget.
func PlutusScript(
	t testing.TB,
	version [3]uint32,
) common.Script {
	t.Helper()
	var body syn.Term[syn.DeBruijn] = &syn.Constant{
		Con: &syn.Integer{Inner: big.NewInt(1)},
	}
	argumentCount := 1
	programVersion := [3]uint32{1, 1, 0}
	if version == lang.LanguageVersionV1 || version == lang.LanguageVersionV2 {
		argumentCount = 3
		programVersion = [3]uint32{1, 0, 0}
	}
	for range argumentCount {
		body = &syn.Lambda[syn.DeBruijn]{Body: body}
	}
	flat, err := syn.Encode(&syn.Program[syn.DeBruijn]{
		Version: programVersion,
		Term:    body,
	})
	if err != nil {
		t.Fatalf("encode Plutus program: %v", err)
	}
	wrapper, err := cbor.Encode(flat)
	if err != nil {
		t.Fatalf("encode Plutus script wrapper: %v", err)
	}
	switch version {
	case lang.LanguageVersionV1:
		return common.PlutusV1Script(wrapper)
	case lang.LanguageVersionV2:
		return common.PlutusV2Script(wrapper)
	case lang.LanguageVersionV3:
		return common.PlutusV3Script(wrapper)
	case lang.LanguageVersionV4:
		return common.PlutusV4Script(wrapper)
	default:
		t.Fatalf("unsupported Plutus language version %v", version)
		return nil
	}
}
