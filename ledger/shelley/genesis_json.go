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

package shelley

import (
	"encoding/json"
	"errors"
	"fmt"

	gj "github.com/blinklabs-io/gouroboros/internal/genesisjson"
)

// validateShelleyGenesisJSON checks a Shelley genesis document against the
// FromJSON ShelleyGenesis instance in cardano-ledger
// (eras/shelley/impl/src/Cardano/Ledger/Shelley/Genesis.hs): fourteen keys are
// required, staking and extraConfig are optional, and unknown keys are
// ignored. Value ranges follow the field types of ShelleyGenesis and of
// ShelleyPParams (Shelley/PParams.hs).
func validateShelleyGenesisJSON(data []byte) error {
	return gj.Validate(
		"",
		data,
		gj.Required("systemStart", gj.Text()),
		gj.Required("networkMagic", gj.Uint(32)),
		// Network derives FromJSON generically: the constructor name.
		gj.Required("networkId", gj.OneOf("Mainnet", "Testnet")),
		gj.Required(
			"activeSlotsCoeff",
			gj.Interval(gj.PositiveUnitInterval),
		),
		gj.Required("securityParam", gj.NonZeroUint64()),
		gj.Required("epochLength", gj.Uint(64)),
		gj.Required("slotsPerKESPeriod", gj.Uint(64)),
		gj.Required("maxKESEvolutions", gj.Uint(64)),
		gj.Required("slotLength", gj.Number()),
		gj.Required("updateQuorum", gj.Uint(64)),
		gj.Required("maxLovelaceSupply", gj.Uint(64)),
		gj.Required("protocolParams", gj.Object(shelleyPParamsFields()...)),
		// GenDelegPair requires both keys.
		gj.Required("genDelegs", gj.MapOf(gj.Object(
			gj.Required("delegate", gj.Text()),
			gj.Required("vrf", gj.Text()),
		))),
		gj.Required("initialFunds", gj.AnyObject()),
		gj.Optional("staking", gj.Object(
			gj.Required("pools", gj.AnyObject()),
			gj.Required("stake", gj.AnyObject()),
		)),
		gj.Optional("extraConfig", validateExtraConfig),
	)
}

// shelleyPParamsFields mirrors FromJSON LegacyJSONPParams
// (Shelley/Genesis.hs); the field types come from ShelleyPParams.
func shelleyPParamsFields() []gj.Field {
	return []gj.Field{
		gj.Required("minFeeA", gj.Uint(64)),
		gj.Required("minFeeB", gj.Uint(64)),
		gj.Required("maxBlockBodySize", gj.Uint(32)),
		gj.Required("maxTxSize", gj.Uint(32)),
		gj.Required("maxBlockHeaderSize", gj.Uint(16)),
		gj.Required("keyDeposit", gj.Uint(64)),
		gj.Required("poolDeposit", gj.Uint(64)),
		gj.Required("eMax", gj.Uint(32)),
		gj.Required("nOpt", gj.Uint(16)),
		gj.Required("a0", gj.Interval(gj.NonNegativeInterval)),
		gj.Required("rho", gj.Interval(gj.UnitInterval)),
		gj.Required("tau", gj.Interval(gj.UnitInterval)),
		gj.Required("decentralisationParam", gj.Interval(gj.UnitInterval)),
		gj.Required("extraEntropy", gj.AnyObject()),
		// The reference bounds major by the era's MaxVersion, which moves
		// with each era; only the Word32 range is enforced here.
		gj.Required("protocolVersion", gj.Object(
			gj.Required("major", gj.Uint(32)),
			gj.Required("minor", gj.Uint(32)),
		)),
		gj.Optional("minUTxOValue", gj.Uint(64)),
		gj.Optional("minPoolCost", gj.Uint(64)),
	}
}

// validateExtraConfig mirrors FromJSON ShelleyExtraConfig and InjectionData.
// Every section is optional. The reference also accepts a file-backed
// injection, which this package does not load; refusing it avoids silently
// dropping injected state.
func validateExtraConfig(path string, raw json.RawMessage) error {
	for _, section := range []string{
		"initialFunds",
		"stakePools",
		"stakeCredentials",
	} {
		injection := gj.Optional(
			section,
			func(p string, v json.RawMessage) error {
				var keys map[string]json.RawMessage
				if err := json.Unmarshal(v, &keys); err != nil {
					return errors.New("expected a JSON object")
				}
				if _, ok := keys["file"]; ok {
					return fmt.Errorf(
						"%s: file-backed injection is not supported",
						p,
					)
				}
				return nil
			},
		)
		if err := gj.Validate(path, raw, injection); err != nil {
			return err
		}
	}
	return nil
}
