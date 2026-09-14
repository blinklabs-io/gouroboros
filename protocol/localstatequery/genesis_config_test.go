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

package localstatequery_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
)

// GetGenesisConfig has two wire layouts. Node-to-client protocol version 21
// (ShelleyNodeToClientVersion13) switched the reply from a vendored copy of an
// older ledger serialisation to the ledger's current one. The two differ in
// arity at both levels, which is what makes a single decoder possible:
//
//	ShelleyGenesis  15 fields (legacy)  ->  16 fields (adds sgExtraConfig)
//	PParams         18 fields (legacy)  ->  17 fields (protocol version became
//	                                        a nested pair rather than two
//	                                        adjacent fields)
//
// A node that negotiates 21 or higher and still emits the legacy layout is not
// merely returning stale field names: the client reads sgGenDelegs where the
// node wrote sgProtocolParams, so every field from the protocol parameters
// onwards is silently wrong.

// genesisProtocolParamsLegacy is the pre-21 PParams layout, with the protocol
// version occupying two adjacent fields.
func genesisProtocolParamsLegacy() []any {
	return []any{
		44,             // minFeeA
		155381,         // minFeeB
		65536,          // maxBlockBodySize
		16384,          // maxTxSize
		1100,           // maxBlockHeaderSize
		2000000,        // keyDeposit
		500000000,      // poolDeposit
		18,             // eMax
		150,            // nOpt
		[]any{3, 10},   // a0
		[]any{3, 1000}, // rho
		[]any{2, 10},   // tau
		[]any{0, 1},    // decentralizationParam
		[]any{0},       // extraEntropy (neutral nonce)
		10,             // protocol version major
		3,              // protocol version minor
		1000000,        // minUTxOValue
		340000000,      // minPoolCost
	}
}

// genesisProtocolParamsCurrent is the same parameters in the layout a node
// negotiating protocol version 21 or higher sends: one field shorter, with the
// protocol version nested.
func genesisProtocolParamsCurrent() []any {
	legacy := genesisProtocolParamsLegacy()
	current := make([]any, 0, len(legacy)-1)
	current = append(current, legacy[:14]...)
	current = append(current, []any{legacy[14], legacy[15]})
	current = append(current, legacy[16:]...)
	return current
}

func genesisConfigCommonPrefix(pparams []any) []any {
	return []any{
		[]any{2017, 254, int64(71856000000000)}, // system start
		42,                                      // network magic
		0,                                       // network id
		[]any{1, 20},                            // active slots coeff
		2160,                                    // security param
		432000,                                  // epoch length
		129600,                                  // slots per KES period
		62,                                      // max KES evolutions
		1,                                       // slot length
		5,                                       // update quorum
		int64(45000000000000000),                // max lovelace supply
		pparams,
		map[any]any{},                 // genesis delegates
		[]any{},                       // initial funds
		[]any{[]any{}, map[any]any{}}, // staking
	}
}

func encodeGenesisConfigLegacy(t *testing.T) []byte {
	t.Helper()
	encoded, err := cbor.Encode(
		genesisConfigCommonPrefix(genesisProtocolParamsLegacy()),
	)
	if err != nil {
		t.Fatalf("encoding legacy genesis fixture: %v", err)
	}
	return encoded
}

func encodeGenesisConfigCurrent(t *testing.T) []byte {
	t.Helper()
	fields := genesisConfigCommonPrefix(genesisProtocolParamsCurrent())
	// sgExtraConfig is a StrictMaybe, so an absent value is the empty list.
	fields = append(fields, []any{})
	encoded, err := cbor.Encode(fields)
	if err != nil {
		t.Fatalf("encoding current genesis fixture: %v", err)
	}
	return encoded
}

// assertGenesisConfigFields checks the fields that sit after the protocol
// parameters, since those are the ones a mis-detected layout shifts.
func assertGenesisConfigFields(
	t *testing.T,
	result localstatequery.GenesisConfigResult,
) {
	t.Helper()
	if result.NetworkMagic != 42 {
		t.Errorf("network magic: got %d, want 42", result.NetworkMagic)
	}
	if result.MaxLovelaceSupply != 45000000000000000 {
		t.Errorf(
			"max lovelace supply: got %d, want 45000000000000000",
			result.MaxLovelaceSupply,
		)
	}
	if result.ProtocolParams.MinFeeA != 44 {
		t.Errorf("minFeeA: got %d, want 44", result.ProtocolParams.MinFeeA)
	}
	if result.ProtocolParams.ProtocolVersionMajor != 10 {
		t.Errorf(
			"protocol version major: got %d, want 10",
			result.ProtocolParams.ProtocolVersionMajor,
		)
	}
	if result.ProtocolParams.ProtocolVersionMinor != 3 {
		t.Errorf(
			"protocol version minor: got %d, want 3",
			result.ProtocolParams.ProtocolVersionMinor,
		)
	}
	if result.ProtocolParams.MinUTxOValue != 1000000 {
		t.Errorf(
			"minUTxOValue: got %d, want 1000000",
			result.ProtocolParams.MinUTxOValue,
		)
	}
	if result.ProtocolParams.MinPoolCost != 340000000 {
		t.Errorf(
			"minPoolCost: got %d, want 340000000",
			result.ProtocolParams.MinPoolCost,
		)
	}
	if len(result.GenDelegs) == 0 {
		t.Error("genesis delegates were not decoded")
	}
}

// TestGenesisConfigResultDecodesLegacyLayout pins the existing behaviour so the
// version-21 work cannot regress clients talking to an older node.
func TestGenesisConfigResultDecodesLegacyLayout(t *testing.T) {
	var result localstatequery.GenesisConfigResult
	if _, err := cbor.Decode(encodeGenesisConfigLegacy(t), &result); err != nil {
		t.Fatalf("decoding legacy genesis config: %v", err)
	}
	assertGenesisConfigFields(t, result)
	if result.ExtraConfig != nil {
		t.Errorf(
			"legacy layout carries no extra config, got %v",
			result.ExtraConfig,
		)
	}
}

// TestGenesisConfigResultDecodesCurrentLayout covers the reply a node sends once
// node-to-client protocol version 21 is negotiated.
func TestGenesisConfigResultDecodesCurrentLayout(t *testing.T) {
	var result localstatequery.GenesisConfigResult
	if _, err := cbor.Decode(encodeGenesisConfigCurrent(t), &result); err != nil {
		t.Fatalf("decoding current genesis config: %v", err)
	}
	assertGenesisConfigFields(t, result)
	if result.ExtraConfig == nil {
		t.Error("current layout carries extra config, got none")
	}
}

// TestGenesisConfigResultRejectsUnknownArity keeps the decoder from quietly
// accepting a record it cannot map, which would hand callers zero values that
// look like real parameters.
func TestGenesisConfigResultRejectsUnknownArity(t *testing.T) {
	encoded, err := cbor.Encode([]any{1, 2, 3})
	if err != nil {
		t.Fatalf("encoding fixture: %v", err)
	}
	var result localstatequery.GenesisConfigResult
	if _, err := cbor.Decode(encoded, &result); err == nil {
		t.Error("expected an error for a genesis record of unknown arity")
	}
}

// TestGenesisConfigProtocolParamsRejectsUnknownArity is the same guard one level
// down, where a shifted decode is just as silent.
func TestGenesisConfigProtocolParamsRejectsUnknownArity(t *testing.T) {
	encoded, err := cbor.Encode([]any{1, 2, 3})
	if err != nil {
		t.Fatalf("encoding fixture: %v", err)
	}
	var result localstatequery.GenesisConfigResultProtocolParameters
	if _, err := cbor.Decode(encoded, &result); err == nil {
		t.Error("expected an error for protocol parameters of unknown arity")
	}
}

// TestGenesisConfigResultRoundTripsLegacyLayout and its current-layout twin pin
// the encoder to the layout the value was decoded from. Re-encoding a reply in
// the other layout would corrupt it in exactly the way described above, and
// byte equality is the only assertion that catches a field moving.
func TestGenesisConfigResultRoundTripsLegacyLayout(t *testing.T) {
	original := encodeGenesisConfigLegacy(t)
	var result localstatequery.GenesisConfigResult
	if _, err := cbor.Decode(original, &result); err != nil {
		t.Fatalf("decoding legacy genesis config: %v", err)
	}
	reencoded, err := cbor.Encode(result)
	if err != nil {
		t.Fatalf("re-encoding legacy genesis config: %v", err)
	}
	if !bytes.Equal(original, reencoded) {
		t.Errorf(
			"legacy round trip changed the encoding:\n got %x\nwant %x",
			reencoded,
			original,
		)
	}
}

func TestGenesisConfigResultRoundTripsCurrentLayout(t *testing.T) {
	original := encodeGenesisConfigCurrent(t)
	var result localstatequery.GenesisConfigResult
	if _, err := cbor.Decode(original, &result); err != nil {
		t.Fatalf("decoding current genesis config: %v", err)
	}
	reencoded, err := cbor.Encode(result)
	if err != nil {
		t.Fatalf("re-encoding current genesis config: %v", err)
	}
	if !bytes.Equal(original, reencoded) {
		t.Errorf(
			"current round trip changed the encoding:\n got %x\nwant %x",
			reencoded,
			original,
		)
	}
}

// TestGenesisConfigProtocolParamsEncodeCurrentByDefault covers a value built in
// Go rather than decoded: new callers should get the layout a node negotiating
// version 21 or higher expects.
func TestGenesisConfigProtocolParamsEncodeCurrentByDefault(t *testing.T) {
	encoded, err := cbor.Encode(
		localstatequery.GenesisConfigResultProtocolParameters{
			ProtocolVersionMajor: 10,
			ProtocolVersionMinor: 3,
		},
	)
	if err != nil {
		t.Fatalf("encoding protocol parameters: %v", err)
	}
	listLen, err := cbor.ListLength(encoded)
	if err != nil {
		t.Fatalf("reading list length: %v", err)
	}
	if listLen != 17 {
		t.Errorf("expected the 17-field current layout, got %d fields", listLen)
	}
}
