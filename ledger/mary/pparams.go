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

package mary

import (
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

type MaryProtocolParameters struct {
	allegra.AllegraProtocolParameters
}

func (p *MaryProtocolParameters) Update(
	paramUpdate *MaryProtocolParameterUpdate,
) {
	p.AllegraProtocolParameters.Update(
		&paramUpdate.AllegraProtocolParameterUpdate,
	)
}

type MaryProtocolParameterUpdate struct {
	allegra.AllegraProtocolParameterUpdate
}

func (u *MaryProtocolParameterUpdate) UnmarshalCBOR(data []byte) error {
	return u.UnmarshalCbor(data, u)
}

func (p *MaryProtocolParameters) Utxorpc() *cardano.PParams {
	return p.AllegraProtocolParameters.Utxorpc()
}
