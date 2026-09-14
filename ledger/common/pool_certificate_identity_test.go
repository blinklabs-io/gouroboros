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

package common_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// StakePoolParams derives Eq/Ord with Set-valued owners and an account address
// containing both network and credential. Certificate identity uses those
// decoded values rather than the original owner order or rational encoding.
func TestPoolCertificateOwnerSetIdentity(t *testing.T) {
	ownerA := make([]byte, 28)
	ownerB := make([]byte, 28)
	ownerB[0] = 1
	makeCert := func(owners any, header byte) []any {
		reward := make([]byte, 29)
		reward[0] = header
		return []any{3, make([]byte, 28), make([]byte, 32), 1, 1,
			cbor.Tag{
				Number:  30,
				Content: []uint{1, 2},
			}, reward, owners, []any{}, nil}
	}
	equivalentMargin := makeCert([][]byte{ownerA, ownerB}, 0xe0)
	equivalentMargin[5] = cbor.Tag{Number: 30, Content: []uint{2, 4}}
	distinctMargin := makeCert([][]byte{ownerA, ownerB}, 0xe0)
	distinctMargin[5] = cbor.Tag{Number: 30, Content: []uint{1, 3}}
	for name, tc := range map[string]struct {
		second    []any
		duplicate bool
	}{
		"identical":                       {makeCert([][]byte{ownerA, ownerB}, 0xe0), true},
		"reordered owners":                {makeCert([][]byte{ownerB, ownerA}, 0xe0), true},
		"tagged reordered owners":         {makeCert(cbor.Tag{Number: 258, Content: [][]byte{ownerB, ownerA}}, 0xe0), true},
		"equivalent margin":               {equivalentMargin, true},
		"distinct margin":                 {distinctMargin, false},
		"distinct owners":                 {makeCert([][]byte{ownerA}, 0xe0), false},
		"distinct reward credential type": {makeCert([][]byte{ownerA, ownerB}, 0xf0), false},
		"distinct reward network":         {makeCert([][]byte{ownerA, ownerB}, 0xe1), false},
	} {
		t.Run(name, func(t *testing.T) {
			for _, tagged := range []bool{false, true} {
				encoding := "untagged"
				certs := any(
					[]any{makeCert([][]byte{ownerA, ownerB}, 0xe0), tc.second},
				)
				if tagged {
					encoding = "tagged"
					certs = cbor.Tag{Number: 258, Content: certs}
				}
				t.Run(encoding, func(t *testing.T) {
					body, err := cbor.Encode(map[uint]any{4: certs})
					require.NoError(t, err)
					for era, newBody := range orderedSetCertificateTransactionBodyDecoders() {
						t.Run(era, func(t *testing.T) {
							err := newBody().UnmarshalCBOR(body)
							if tc.duplicate {
								var target common.DuplicateCertificateError
								require.ErrorAs(
									t,
									err,
									&target,
									"equivalent pool certificates must be rejected",
								)
							} else {
								require.NoError(t, err, "distinct pool registrations must remain distinct")
							}
						})
					}
					if !tagged {
						for era, newBody := range preConwayTransactionBodyDecoders() {
							t.Run(
								era,
								func(t *testing.T) { require.NoError(t, newBody().UnmarshalCBOR(body)) },
							)
						}
					}
				})
			}
		})
	}
	// Calling the public identity API must preserve original bytes and owner order.
	encoded, err := cbor.Encode(makeCert([][]byte{ownerB, ownerA}, 0xe0))
	require.NoError(t, err)
	var cert common.PoolRegistrationCertificate
	_, err = cbor.Decode(encoded, &cert)
	require.NoError(t, err)
	ownersBefore := append([]common.AddrKeyHash(nil), cert.PoolOwners...)
	_, err = common.CertificateLogicalKey(&cert)
	require.NoError(t, err)
	require.Equal(t, ownersBefore, cert.PoolOwners)
	require.Equal(t, encoded, cert.Cbor())
	knownKey, err := common.CertificateLogicalKey(&cert)
	require.NoError(t, err)
	unknown := cert
	unknown.SetCbor(encoded)
	_, known := unknown.RewardAccountNetworkId()
	require.False(t, known)
	unknownKey, err := common.CertificateLogicalKey(&unknown)
	require.NoError(t, err)
	require.NotEqual(t, knownKey, unknownKey,
		"unknown reward network must not identify as known testnet")
	unknownCopy := unknown
	unknownCopyKey, err := common.CertificateLogicalKey(&unknownCopy)
	require.NoError(t, err)
	require.Equal(t, unknownKey, unknownCopyKey)
}
