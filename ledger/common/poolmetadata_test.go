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

package common_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	common "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

// Tests for stake pool metadata (CIP-0006)
func TestPoolMetadataUtxorpc(t *testing.T) {
	hash := common.NewBlake2b256([]byte{1, 2, 3, 4})
	pm := &common.PoolMetadata{
		Url:  "https://example.com/poolmeta.json",
		Hash: hash,
	}

	rpc, err := pm.Utxorpc()
	assert.NoError(t, err)
	if !assert.NotNil(t, rpc) {
		t.FailNow()
	}
	assert.Equal(t, pm.Url, rpc.Url)
	assert.True(t, bytes.Equal(pm.Hash[:], rpc.Hash))
}

func TestPoolRegistrationCertificateUtxorpcIncludesMetadata(t *testing.T) {
	hash := common.NewBlake2b256([]byte{9, 8, 7, 6})
	pm := &common.PoolMetadata{
		Url:  "https://example.org/pm.json",
		Hash: hash,
	}

	cert := &common.PoolRegistrationCertificate{
		CertType:      1,
		Operator:      common.PoolKeyHash{1, 2, 3},
		VrfKeyHash:    common.VrfKeyHash(common.NewBlake2b256([]byte{0x0})),
		Pledge:        0,
		Cost:          0,
		Margin:        common.NewGenesisRat(0, 1),
		RewardAccount: common.AddrKeyHash{},
		PoolOwners:    []common.AddrKeyHash{},
		Relays:        []common.PoolRelay{},
		PoolMetadata:  pm,
	}

	rpcCert, err := cert.Utxorpc()
	assert.NoError(t, err)
	if !assert.NotNil(t, rpcCert) {
		t.FailNow()
	}

	pr, ok := rpcCert.Certificate.(*utxorpc.Certificate_PoolRegistration)
	assert.True(t, ok, "expected PoolRegistration certificate type")
	if !assert.NotNil(t, pr.PoolRegistration.PoolMetadata) {
		t.FailNow()
	}
	assert.Equal(t, pm.Url, pr.PoolRegistration.PoolMetadata.Url)
	assert.True(
		t,
		bytes.Equal(pm.Hash[:], pr.PoolRegistration.PoolMetadata.Hash),
	)
}

func TestPoolMetadataCbor(t *testing.T) {
	// Test CBOR encoding/decoding for CIP-0006 compliance
	hash := common.NewBlake2b256([]byte{1, 2, 3, 4, 5})
	pm := &common.PoolMetadata{
		Url:  "https://pool.example.com/metadata.json",
		Hash: hash,
	}

	// Encode to CBOR
	cborData, err := cbor.Encode(pm)
	assert.NoError(t, err)
	assert.NotEmpty(t, cborData)

	// Decode back from CBOR
	var decoded common.PoolMetadata
	_, err = cbor.Decode(cborData, &decoded)
	assert.NoError(t, err)

	// Verify fields match
	assert.Equal(t, pm.Url, decoded.Url)
	assert.Equal(t, pm.Hash, decoded.Hash)

	// Re-encode and verify byte-identical round-trip
	reEncodedData, err := cbor.Encode(&decoded)
	assert.NoError(t, err)
	assert.Equal(t, cborData, reEncodedData, "CBOR round-trip should be byte-identical")
}
