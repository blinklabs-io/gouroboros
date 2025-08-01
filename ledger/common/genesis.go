// Copyright 2024 Blink Labs Software
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

package common

import (
	"encoding/json"
	"fmt"
	"math/big"
)

// GenesisRat is a wrapper to big.Rat that allows for unmarshaling from a bare float from JSON
type GenesisRat struct {
	*big.Rat
}

func (r *GenesisRat) UnmarshalJSON(data []byte) error {
	// Try as ratio
	var tmpData struct {
		Numerator   int64 `json:"numerator"`
		Denominator int64 `json:"denominator"`
	}
	if err := json.Unmarshal(data, &tmpData); err == nil {
		r.Rat = big.NewRat(tmpData.Numerator, tmpData.Denominator)
		return nil
	}
	// Try as decimal value
	r.Rat = new(big.Rat)
	if _, ok := r.SetString(string(data)); !ok {
		return fmt.Errorf("math/big: cannot unmarshal %q into a *big.Rat", data)
	}
	return nil
}
