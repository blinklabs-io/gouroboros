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

package common

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// rawDrepCbor builds a drep array of an arbitrary shape. Drep.MarshalCBOR
// only emits the legal forms, so malformed fixtures come from the generic
// encoder instead.
func rawDrepCbor(t *testing.T, items ...any) []byte {
	t.Helper()
	encoded, err := cbor.Encode(items)
	if err != nil {
		t.Fatalf("encoding drep fixture: %v", err)
	}
	return encoded
}

// The Conway and Dijkstra CDDL both give drep exactly four forms:
//
//	drep = [0, addr_keyhash// 1, script_hash// 2// 3]
//
// with addr_keyhash and script_hash aliased to hash28. cardano-ledger decodes
// the hash through a fixed-size codec that fails on any other length, and
// decodes the predefined options through decodeRecordSum, which fails on a
// trailing element. Drep.UnmarshalJSON already rejects a credential that is
// not 28 bytes; the CBOR path did not.
func TestDrepUnmarshalCBORRejectsNonCddlShapes(t *testing.T) {
	t.Parallel()
	hash28 := bytes.Repeat([]byte{0xab}, Blake2b224Size)
	testCases := []struct {
		name  string
		items []any
	}{
		{"key hash, empty credential", []any{DrepTypeAddrKeyHash, []byte{}}},
		{
			"key hash, 1-byte credential",
			[]any{DrepTypeAddrKeyHash, []byte{0x01}},
		},
		{
			"key hash, 27-byte credential",
			[]any{DrepTypeAddrKeyHash, bytes.Repeat([]byte{0xab}, 27)},
		},
		{
			"key hash, 29-byte credential",
			[]any{DrepTypeAddrKeyHash, bytes.Repeat([]byte{0xab}, 29)},
		},
		{
			"script hash, 2-byte credential",
			[]any{DrepTypeScriptHash, []byte{0x01, 0x02}},
		},
		{
			"script hash, 32-byte credential",
			[]any{DrepTypeScriptHash, bytes.Repeat([]byte{0xcd}, 32)},
		},
		{"key hash, no credential", []any{DrepTypeAddrKeyHash}},
		{
			"key hash, trailing element",
			[]any{DrepTypeAddrKeyHash, hash28, 1},
		},
		{"abstain, trailing value", []any{DrepTypeAbstain, 123}},
		{"no confidence, trailing value", []any{DrepTypeNoConfidence, hash28}},
		{"empty array", []any{}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var drep Drep
			err := drep.UnmarshalCBOR(rawDrepCbor(t, testCase.items...))
			if err == nil {
				t.Fatalf(
					"decoded %v without error; the CDDL admits only"+
						" [0, hash28], [1, hash28], [2] and [3]",
					testCase.items,
				)
			}
		})
	}
}

// The rejections above must not come from rejecting everything.
func TestDrepUnmarshalCBORAcceptsCddlShapes(t *testing.T) {
	t.Parallel()
	hash28 := bytes.Repeat([]byte{0xab}, Blake2b224Size)
	testCases := []struct {
		name     string
		items    []any
		drepType int
		expected []byte
	}{
		{
			"key hash",
			[]any{DrepTypeAddrKeyHash, hash28},
			DrepTypeAddrKeyHash,
			hash28,
		},
		{
			"script hash",
			[]any{DrepTypeScriptHash, hash28},
			DrepTypeScriptHash,
			hash28,
		},
		{"abstain", []any{DrepTypeAbstain}, DrepTypeAbstain, nil},
		{
			"no confidence",
			[]any{DrepTypeNoConfidence},
			DrepTypeNoConfidence,
			nil,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var drep Drep
			err := drep.UnmarshalCBOR(rawDrepCbor(t, testCase.items...))
			if err != nil {
				t.Fatalf("decoding a CDDL-legal drep: %v", err)
			}
			if drep.Type != testCase.drepType {
				t.Errorf(
					"drep type: got %d, want %d",
					drep.Type,
					testCase.drepType,
				)
			}
			if !bytes.Equal(drep.Credential, testCase.expected) {
				t.Errorf(
					"drep credential: got %x, want %x",
					drep.Credential,
					testCase.expected,
				)
			}
		})
	}
}
