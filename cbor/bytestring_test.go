// Copyright 2023 Blink Labs Software
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
package cbor

import (
	"encoding/json"
	"testing"
)

// Test the String method to ensure it properly converts ByteString to hex.
func TestByteString_String(t *testing.T) {
	data := []byte("blinklabs") // "blinklabs" as bytes
	bs := NewByteString(data)

	expected := "626c696e6b6c616273" // "blinklabs" in hex
	actual := bs.String()

	if actual != expected {
		t.Errorf("expected %s but got %s", expected, actual)
	}
}

// Test the MarshalJSON method to ensure it properly marshals ByteString to JSON as hex.
func TestByteString_MarshalJSON(t *testing.T) {
	data := []byte("blinklabs") // "blinklabs" as bytes
	bs := NewByteString(data)

	jsonData, err := json.Marshal(bs)
	if err != nil {
		t.Fatalf("failed to marshal ByteString: %v", err)
	}

	// Expected JSON result, hex-encoded string
	expectedJSON := `"626c696e6b6c616273"` // "blinklabs" in hex

	if string(jsonData) != expectedJSON {
		t.Errorf("expected %s but got %s", expectedJSON, string(jsonData))
	}
}

// Test NewByteString to ensure it properly wraps the byte slice
func TestNewByteString(t *testing.T) {
	data := []byte{0x41, 0x42, 0x43} // "ABC" in hex
	bs := NewByteString(data)

	if string(bs.Bytes()) != "ABC" {
		t.Errorf("expected ABC but got %s", string(bs.Bytes()))
	}
}
