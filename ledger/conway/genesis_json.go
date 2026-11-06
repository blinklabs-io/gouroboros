// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package conway

import (
	gj "github.com/blinklabs-io/gouroboros/internal/genesisjson"
)

// validateConwayGenesisJSON checks a Conway genesis document against the
// FromJSON ConwayGenesis instance in cardano-ledger
// (eras/conway/impl/src/Cardano/Ledger/Conway/Genesis.hs). The protocol
// parameter keys are the required FromJSON UpgradeConwayPParams keys
// (Conway/PParams.hs), constitution and committee are required, and delegs,
// initialDReps and extraConfig are optional. Unknown keys are ignored.
func validateConwayGenesisJSON(data []byte) error {
	unit := gj.Interval(gj.UnitInterval)
	return gj.Validate(
		"",
		data,
		// PoolVotingThresholds and DRepVotingThresholds: every key required,
		// each a UnitInterval.
		gj.Required("poolVotingThresholds", gj.Object(
			gj.Required("motionNoConfidence", unit),
			gj.Required("committeeNormal", unit),
			gj.Required("committeeNoConfidence", unit),
			gj.Required("hardForkInitiation", unit),
			gj.Required("ppSecurityGroup", unit),
		)),
		gj.Required("dRepVotingThresholds", gj.Object(
			gj.Required("motionNoConfidence", unit),
			gj.Required("committeeNormal", unit),
			gj.Required("committeeNoConfidence", unit),
			gj.Required("updateToConstitution", unit),
			gj.Required("hardForkInitiation", unit),
			gj.Required("ppNetworkGroup", unit),
			gj.Required("ppEconomicGroup", unit),
			gj.Required("ppTechnicalGroup", unit),
			gj.Required("ppGovGroup", unit),
			gj.Required("treasuryWithdrawal", unit),
		)),
		gj.Required("committeeMinSize", gj.Uint(16)),
		// EpochInterval is a Word32.
		gj.Required("committeeMaxTermLength", gj.Uint(32)),
		gj.Required("govActionLifetime", gj.Uint(32)),
		gj.Required("govActionDeposit", gj.Uint(64)),
		gj.Required("dRepDeposit", gj.Uint(64)),
		gj.Required("dRepActivity", gj.Uint(32)),
		gj.Required(
			"minFeeRefScriptCostPerByte",
			gj.Interval(gj.NonNegativeInterval),
		),
		// parseCostModelAsArray.
		gj.Required("plutusV3CostModel", gj.Array()),
		// FromJSON Constitution and FromJSON Anchor.
		gj.Required("constitution", gj.Object(
			gj.Required("anchor", gj.Object(
				gj.Required("url", gj.Text()),
				gj.Required("dataHash", gj.Text()),
			)),
			gj.Optional("script", gj.Text()),
		)),
		// FromJSON Committee: members map to an EpochNo (Word64).
		gj.Required("committee", gj.Object(
			gj.Required("members", gj.MapOf(gj.Uint(64))),
			gj.Required("threshold", unit),
		)),
		gj.Optional("delegs", gj.AnyObject()),
		gj.Optional("initialDReps", gj.AnyObject()),
		gj.Optional("extraConfig", gj.AnyObject()),
	)
}
