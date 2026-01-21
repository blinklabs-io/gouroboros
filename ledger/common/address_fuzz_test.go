// Copyright 2024 Cardano Foundation
// Copyright 2025 Blink Labs Software
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

//go:build go1.18

package common

import "testing"

func FuzzNewAddressFromBytes(f *testing.F) {
	// Seed with valid address bytes
	f.Add(
		[]byte{0x01, 0x02, 0x03, 0x04, 0x05},
	) // Minimal valid address
	f.Add(
		[]byte{
			0x61,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
			0x00,
		},
	) // Shelley address format

	f.Fuzz(func(t *testing.T, data []byte) {
		// Should not panic on any input - that's the test
		_, _ = NewAddressFromBytes(data)
	})
}

func FuzzNewAddress(f *testing.F) {
	// Seed with valid address strings
	f.Add(
		"addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd",
	) // Valid Shelley address
	f.Add(
		"Ae2tdPwUPEZ18ZjTLnLVr9CEvUEUXwFhNVyRn867GQ",
	) // Valid Byron address
	f.Add(
		"invalid_address_string",
	) // Invalid string

	f.Fuzz(func(t *testing.T, addr string) {
		// Should not panic on any input - that's the test
		_, _ = NewAddress(addr)
	})
}

func FuzzNewScriptHashFromBech32(f *testing.F) {
	// Seed with valid script hash strings
	f.Add(
		"script1uqh2r0m8k2s0z8h2m2g0z8h2m2g0z8h2m2g0z8h2m2g",
	) // Valid script hash
	f.Add(
		"invalid_script_hash",
	) // Invalid string

	f.Fuzz(func(t *testing.T, scriptHash string) {
		// Should not panic on any input - that's the test
		_, _ = NewScriptHashFromBech32(scriptHash)
	})
}
