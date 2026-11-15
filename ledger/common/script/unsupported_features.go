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

package script

import (
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
)

// UnsupportedFeatureError reports a transaction feature that the Plutus V1,
// V2 and V3 script contexts cannot represent.
type UnsupportedFeatureError struct {
	Feature string
}

func (e UnsupportedFeatureError) Error() string {
	return "plutus V1-V3 script context does not support " + e.Feature
}

// V1V3FeatureChecker is implemented by transactions whose body can carry
// fields that the Plutus V1 to V3 script contexts have no representation for.
// CheckPlutusV1V3Features returns an UnsupportedFeatureError for the first such
// field that is present.
type V1V3FeatureChecker interface {
	CheckPlutusV1V3Features() error
}

// CheckPlutusV1V3Features reports whether tx can be translated into a Plutus
// V1, V2 or V3 script context. Transactions that do not implement
// V1V3FeatureChecker have no such fields and always pass.
func CheckPlutusV1V3Features(tx lcommon.Transaction) error {
	if checker, ok := tx.(V1V3FeatureChecker); ok {
		return checker.CheckPlutusV1V3Features()
	}
	return nil
}
