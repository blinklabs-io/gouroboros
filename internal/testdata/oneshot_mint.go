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

// Package testdata provides shared test data for the evaluate regression test.
package testdata

import _ "embed"

// OneshotMintInputsHex is the hex-encoded mainnet one-shot mint transaction's
// 7 spent inputs in Alonzo array form. Decoded from real-inputs-ins.hex
// extracted from /home/ada via Koios /tx_cbor of parent tx 8e8d35cd843660637e73052098b645eaca77099c1a8080d924a35b90a55587f6.
//
//go:embed oneshot_mint_inputs.hex
var OneshotMintInputsHex string

// OneshotMintInputOutputsHex is the hex-encoded mainnet one-shot mint transaction's
// 7 spent outputs in Alonzo array form. Decoded from real-inputs-outs.hex
// extracted from /home/ada via Koios /tx_cbor of parent tx 8e8d35cd...
//
//go:embed oneshot_mint_input_outputs.hex
var OneshotMintInputOutputsHex string

// OneshotMintRefInputHex is the hex-encoded reference input in Babbage MAP form.
// Decoded from real-refs-ins.hex extracted from Koios.
//
//go:embed oneshot_mint_ref_input.hex
var OneshotMintRefInputHex string

// OneshotMintRefOutputHex is the hex-encoded reference output in Babbage MAP form
// with an INLINE datum. Decoded from real-refs-outs.hex extracted from Koios.
//
//go:embed oneshot_mint_ref_output.hex
var OneshotMintRefOutputHex string

// OneshotMintCollateralInputHex is the hex-encoded collateral input.
// Decoded from real-collateral-ins.hex extracted from Koios.
//
//go:embed oneshot_mint_collateral_input.hex
var OneshotMintCollateralInputHex string

// OneshotMintCollateralOutputHex is the hex-encoded collateral output.
// Decoded from real-collateral-outs.hex extracted from Koios.
//
//go:embed oneshot_mint_collateral_output.hex
var OneshotMintCollateralOutputHex string
