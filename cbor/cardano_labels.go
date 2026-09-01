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

package cbor

// CardanoTxBodyLabels maps Conway-era transaction body map keys to names.
// Indices follow the Conway CDDL transaction_body definition. Earlier eras
// use a strict subset of these keys, so the same map can label any era's
// transaction body — the gaps (e.g. 6, 10, 12) reflect keys retired or
// reserved across hard forks.
var CardanoTxBodyLabels = map[int]string{
	0:  "inputs",
	1:  "outputs",
	2:  "fee",
	3:  "ttl",
	4:  "certificates",
	5:  "withdrawals",
	6:  "update",
	7:  "auxiliary_data_hash",
	8:  "validity_interval_start",
	9:  "mint",
	11: "script_data_hash",
	13: "collateral_inputs",
	14: "required_signers",
	15: "network_id",
	16: "collateral_return",
	17: "total_collateral",
	18: "reference_inputs",
	19: "voting_procedures",
	20: "proposal_procedures",
	21: "current_treasury_value",
	22: "donation",
}

// CardanoWitnessLabels maps Conway-era witness set map keys to names.
var CardanoWitnessLabels = map[int]string{
	0: "vkey_witnesses",
	1: "native_scripts",
	2: "bootstrap_witnesses",
	3: "plutus_v1_scripts",
	4: "plutus_data",
	5: "redeemers",
	6: "plutus_v2_scripts",
	7: "plutus_v3_scripts",
}

// CardanoBlockLabels maps block array indices to names. Shelley through Conway
// blocks use the first five entries; Dijkstra adds nullable Leios and Peras
// certificate slots.
var CardanoBlockLabels = []string{
	"header",
	"transaction_bodies",
	"transaction_witnesses",
	"auxiliary_data_set",
	"invalid_transactions",
	"leios_cert",
	"peras_cert",
}

// CardanoTxArrayLabels maps the indices of a serialised transaction array
// ([body, witness_set, is_valid, auxiliary_data]). is_valid was introduced
// in Alonzo; pre-Alonzo transactions only have three elements.
var CardanoTxArrayLabels = []string{
	"body",
	"witness_set",
	"is_valid",
	"auxiliary_data",
}

// CardanoTxOutputLabels maps the keys of a Babbage-and-later post-alonzo
// transaction output map.
var CardanoTxOutputLabels = map[int]string{
	0: "address",
	1: "value",
	2: "datum_option",
	3: "script_ref",
}

// CardanoNativeScriptLabels maps the leading type tag of a native script
// array to a human-readable operator name.
var CardanoNativeScriptLabels = map[int]string{
	0: "script_pubkey",
	1: "script_all",
	2: "script_any",
	3: "script_n_of_k",
	4: "invalid_before",
	5: "invalid_hereafter",
}

// CardanoPlutusConstrTagRanges defines the CBOR tag ranges that Plutus uses
// for compact constructor encoding. Values inside these ranges are
// constructor application; outside, they fall back to RFC 8949 tag form.
const (
	// Compact constructors 0..6 use tags 121..127.
	PlutusConstrAlt1Min = CborTagAlternative1Min
	PlutusConstrAlt1Max = CborTagAlternative1Max
	// Compact constructors 7..127 use tags 1280..1400.
	PlutusConstrAlt2Min = CborTagAlternative2Min
	PlutusConstrAlt2Max = CborTagAlternative2Max
	// General constructor falls back to tag 102 with [index, fields]
	// array per the Plutus CDDL. (Note: the unrelated
	// CborTagAlternative3 constant in tags.go is reserved for a
	// different alternative-data scheme and is not used here.)
	PlutusConstrGeneral uint64 = 102
)

// PlutusConstrFromTag returns the constructor index encoded by a Plutus
// compact-constructor tag, and true if the tag is in either compact range.
// Tags outside the compact ranges (including the general 102 tag) return
// (0, false).
func PlutusConstrFromTag(tag uint64) (uint64, bool) {
	switch {
	case tag >= PlutusConstrAlt1Min && tag <= PlutusConstrAlt1Max:
		return tag - PlutusConstrAlt1Min, true
	case tag >= PlutusConstrAlt2Min && tag <= PlutusConstrAlt2Max:
		return tag - PlutusConstrAlt2Min + 7, true
	}
	return 0, false
}
