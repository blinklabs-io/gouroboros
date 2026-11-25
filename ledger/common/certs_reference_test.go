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
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// The certificates below are written by hand from the conway.cddl rules of
// cardano-ledger 1210d37f (eras/conway/impl/cddl/data/conway.cddl:
// stake_registration, reg_cert, pool_registration, pool_params, relay,
// pool_metadata). Expected verdicts come from the reference decoders cited
// at each test, never from gOuroboros.

func refKeyCred(b byte) string { return "8200581c" + refHash(28, b) }

// refPoolRegistration builds a 10-field pool_registration certificate with
// the given margin, reward account header byte, owners, relays and metadata,
// each already CBOR hex.
func refPoolRegistration(
	margin string,
	rewardHeader string,
	owners string,
	relays string,
	metadata string,
) string {
	return "8a03" +
		"581c" + refHash(28, 0x11) + // operator
		"5820" + refHash(32, 0x22) + // vrf_keyhash
		"1a000f4240" + // pledge
		"1a0001adb0" + // cost
		margin +
		"581d" + rewardHeader + refHash(28, 0x33) +
		owners + relays + metadata
}

const (
	refOwnerA = "581c" + "a1a1a1a1a1a1a1a1a1a1a1a1a1a1" +
		"a1a1a1a1a1a1a1a1a1a1a1a1a1a1"
	refOwnerB = "581c" + "b2b2b2b2b2b2b2b2b2b2b2b2b2b2" +
		"b2b2b2b2b2b2b2b2b2b2b2b2b2b2"
	refRelay1  = "820266" + "72656c617931" // [2, "relay1"]
	refRelay2  = "820266" + "72656c617932" // [2, "relay2"]
	refHalf    = "d81e820102"
	refQuarter = "d81e820204"
)

func decodeRefCertificate(t *testing.T, hexCert string) CertificateWrapper {
	t.Helper()
	raw, err := hex.DecodeString(hexCert)
	if err != nil {
		t.Fatalf("bad hex %q: %v", hexCert, err)
	}
	var cert CertificateWrapper
	if _, err := cbor.Decode(raw, &cert); err != nil {
		t.Fatalf("decode %q: %v", hexCert, err)
	}
	return cert
}

// TestCertificateSetAsReference checks duplicate detection in a Conway
// certificate list. cardano-ledger decodes certs as an OSet (ctbrCerts,
// eras/conway/impl/src/Cardano/Ledger/Conway/TxBody.hs:132) through
// decodeOSet = decodeSetLikeEnforceNoDuplicates
// (libs/cardano-data/src/Data/OSet/Strict.hs:109-110), which fails when two
// decoded TxCert values are equal (derived Eq). Encoding variants that decode
// to the same value are therefore duplicates; any field difference is not.
// Pool owners are a Set (sppOwners, cardano-ledger-core State/StakePool.hs:494)
// so their order is not part of the value, relays are a StrictSeq so theirs
// is, and the margin is a UnitInterval normalised by boundRational
// (BaseTypes.hs:350-361), so 2/4 equals 1/2.
func TestCertificateSetAsReference(t *testing.T) {
	ownersAB := "82" + refOwnerA + refOwnerB
	ownersBA := "82" + refOwnerB + refOwnerA
	relays12 := "82" + refRelay1 + refRelay2
	relays21 := "82" + refRelay2 + refRelay1
	metadata := "82" + "6f" + hex.EncodeToString(
		[]byte("https://x.io/m1"),
	) + "5820" + refHash(32, 0x44)
	pool := refPoolRegistration(refHalf, "e1", ownersAB, relays12, "f6")
	cases := []struct {
		name      string
		a, b      string
		duplicate bool
	}{
		{
			"stake registration repeated",
			"8200" + refKeyCred(0xaa), "8200" + refKeyCred(0xaa), true,
		},
		{
			"stake registration, non-shortest credential tag",
			"8200" + refKeyCred(0xaa),
			"8200" + "821800581c" + refHash(28, 0xaa),
			true,
		},
		{
			"stake registration, non-shortest certificate tag",
			"8200" + refKeyCred(0xaa),
			"821800" + refKeyCred(0xaa),
			true,
		},
		{
			"stake registration, key versus script credential",
			"8200" + refKeyCred(0xaa),
			"8200" + "8201581c" + refHash(28, 0xaa),
			false,
		},
		{
			"stake registration, different credentials",
			"8200" + refKeyCred(0xaa), "8200" + refKeyCred(0xbb), false,
		},
		{
			"reg_cert, deposit in a wider integer head",
			"8307" + refKeyCred(0xaa) + "1a001e8480",
			"8307" + refKeyCred(0xaa) + "1b00000000001e8480",
			true,
		},
		{
			"reg_cert, different deposits",
			"8307" + refKeyCred(0xaa) + "1a001e8480",
			"8307" + refKeyCred(0xaa) + "1a001e8481",
			false,
		},
		{
			"registration forms 0 and 7 are different constructors",
			"8200" + refKeyCred(0xaa),
			"8307" + refKeyCred(0xaa) + "1a001e8480",
			false,
		},
		{"pool registration repeated", pool, pool, true},
		{
			"pool registration, owners reordered",
			pool,
			refPoolRegistration(refHalf, "e1", ownersBA, relays12, "f6"),
			true,
		},
		{
			"pool registration, owners as a tagged set",
			pool,
			refPoolRegistration(
				refHalf,
				"e1",
				"d90102"+ownersAB,
				relays12,
				"f6",
			),
			true,
		},
		{
			"pool registration, unreduced margin",
			pool,
			refPoolRegistration(refQuarter, "e1", ownersAB, relays12, "f6"),
			true,
		},
		{
			"pool registration, relays reordered",
			pool,
			refPoolRegistration(refHalf, "e1", ownersAB, relays21, "f6"),
			false,
		},
		{
			"pool registration, reward account network differs",
			pool,
			refPoolRegistration(refHalf, "e0", ownersAB, relays12, "f6"),
			false,
		},
		{
			"pool registration, reward account script credential",
			pool,
			refPoolRegistration(refHalf, "f1", ownersAB, relays12, "f6"),
			false,
		},
		{
			"pool registration, metadata added",
			pool,
			refPoolRegistration(refHalf, "e1", ownersAB, relays12, metadata),
			false,
		},
		{
			"drep registration, null anchor versus anchor",
			"8410" + refKeyCred(0xaa) + "1a001e8480" + "f6",
			"8410" + refKeyCred(0xaa) + "1a001e8480" + metadata,
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := decodeRefCertificate(t, tc.a)
			b := decodeRefCertificate(t, tc.b)
			keyA, err := CertificateLogicalKey(a.Certificate)
			if err != nil {
				t.Fatalf("key a: %v", err)
			}
			keyB, err := CertificateLogicalKey(b.Certificate)
			if err != nil {
				t.Fatalf("key b: %v", err)
			}
			if got := keyA == keyB; got != tc.duplicate {
				t.Fatalf("keys equal = %v, reference values equal = %v",
					got, tc.duplicate)
			}
			err = ValidateCertificateSet([]CertificateWrapper{a, b})
			var dup DuplicateCertificateError
			if tc.duplicate {
				if !errors.As(err, &dup) || dup.Index != 1 {
					t.Fatalf("want duplicate at index 1, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("distinct certificates rejected: %v", err)
			}
		})
	}
	if _, err := CertificateLogicalKey(nil); err == nil {
		t.Error("nil certificate accepted")
	}
}

// TestPoolRegistrationOwnersAsReference checks the owner set of a pool
// registration decoded from decoder version 9: decodeSet switches to
// decodeSetEnforceNoDuplicates (cardano-ledger-binary
// Decoding/Decoder.hs:943-952), which fails on a repeated element whether or
// not the set carries tag 258. ValidatePoolRegistrationOwners is the Conway
// and Dijkstra decoders' half of that rule. The nine-owner case crosses its
// switch from pairwise comparison to a map.
func TestPoolRegistrationOwnersAsReference(t *testing.T) {
	nineDistinct := "89"
	for i := range 9 {
		nineDistinct += "581c" + refHash(28, byte(0x50+i))
	}
	nineRepeatLast := "89"
	for i := range 8 {
		nineRepeatLast += "581c" + refHash(28, byte(0x50+i))
	}
	nineRepeatLast += "581c" + refHash(28, 0x50)
	cases := []struct {
		name   string
		owners string
		ok     bool
	}{
		{"empty", "80", true},
		{"one", "81" + refOwnerA, true},
		{"two distinct", "82" + refOwnerA + refOwnerB, true},
		{"two distinct tagged", "d9010282" + refOwnerA + refOwnerB, true},
		{"repeated", "82" + refOwnerA + refOwnerA, false},
		{"repeated tagged", "d9010282" + refOwnerA + refOwnerA, false},
		{"repeated, indefinite", "9f" + refOwnerA + refOwnerA + "ff", false},
		{"nine distinct", nineDistinct, true},
		{"nine, first repeated last", nineRepeatLast, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cert := decodeRefCertificate(t, refPoolRegistration(
				refHalf, "e1", tc.owners, "80", "f6",
			))
			err := ValidatePoolRegistrationOwners(
				[]CertificateWrapper{
					decodeRefCertificate(t, "8200"+refKeyCred(0xaa)),
					cert,
				},
			)
			if got := err == nil; got != tc.ok {
				t.Fatalf("accepted = %v, reference = %v (err %v)",
					got, tc.ok, err)
			}
		})
	}
}

// TestPreConwayCertificateTypesAsReference checks the Shelley-Babbage
// certificate constructors: ShelleyTxCert's decoder accepts tags 0-6 and
// fails with invalidKey on any other
// (eras/shelley/impl/src/Cardano/Ledger/Shelley/TxCert.hs:524-536).
func TestPreConwayCertificateTypesAsReference(t *testing.T) {
	for certType := uint(0); certType <= 20; certType++ {
		err := ValidatePreConwayCertificateTypes(
			[]CertificateWrapper{{Type: 0}, {Type: certType}},
		)
		if got, want := err == nil, certType <= 6; got != want {
			t.Errorf("type %d: accepted = %v, reference = %v",
				certType, got, want)
		}
	}
}

// TestPoolUrlAndDnsLengthAsReference checks the text bounds of pool
// metadata URLs and relay DNS names. Url and DnsName decode with
// textDecCBOR 128 from decoder version 9 and textDecCBOR 64 before it
// (libs/cardano-ledger-core/src/Cardano/Ledger/BaseTypes.hs:679-698), and
// textSizeN measures UTF-8 bytes, not characters (lengthWord8, lines
// 645-658).
func TestPoolUrlAndDnsLengthAsReference(t *testing.T) {
	texts := []string{
		strings.Repeat("a", 64),
		strings.Repeat("a", 65),
		strings.Repeat("a", 128),
		strings.Repeat("a", 129),
		strings.Repeat("é", 32), // 64 bytes
		strings.Repeat("é", 33), // 66 bytes, 33 characters
	}
	for _, text := range texts {
		for _, pv := range []uint{2, 7, 8, 9, 10, 12} {
			limit := 64
			if pv >= 9 {
				limit = 128
			}
			want := len(text) <= limit
			name := fmt.Sprintf("%d bytes/pv%d", len(text), pv)
			err := ValidatePoolMetadataForProtocolVersion(
				&PoolMetadata{Url: text},
				pv,
			)
			if got := err == nil; got != want {
				t.Errorf("url %s: accepted = %v, reference = %v",
					name, got, want)
			}
			for _, relayType := range []int{
				PoolRelayTypeSingleHostName,
				PoolRelayTypeMultiHostName,
			} {
				host := text
				err := ValidatePoolRelaysForProtocolVersion(
					[]PoolRelay{{Type: relayType, Hostname: &host}},
					pv,
				)
				if got := err == nil; got != want {
					t.Errorf("dns type %d %s: accepted = %v, reference = %v",
						relayType, name, got, want)
				}
			}
		}
	}
}

// TestPoolRelayIpv6WireOrderAsReference checks the IPv6 relay byte order.
// ipv6ToBytes (cardano-ledger-binary Encoding/Encoder.hs:569-575) writes the
// four 32-bit words of fromIPv6w with putWord32le, so 2001:db8:102:304:
// 506:708:90a:b0c is written as each word's bytes reversed.
func TestPoolRelayIpv6WireOrderAsReference(t *testing.T) {
	network := net.ParseIP("2001:db8:102:304:506:708:90a:b0c")
	wire, _ := hex.DecodeString("b80d012004030201080706050c0b0a09")
	if got := PoolRelayIpv6WireOrder(network); !bytes.Equal(got, wire) {
		t.Fatalf("wire order %x, reference %x", got, wire)
	}
	if got := PoolRelayIpv6WireOrder(net.IP(wire)); !bytes.Equal(
		got,
		network.To16(),
	) {
		t.Fatalf("network order %x, reference %x", got, network.To16())
	}
	if PoolRelayIpv6WireOrder(net.ParseIP("192.0.2.1").To4()) != nil {
		t.Error("four-byte address accepted")
	}
}

// TestPoolMarginAsReference checks the UnitInterval domain of a pool margin:
// boundRational (BaseTypes.hs:350-361) requires the reduced numerator and
// denominator to fit Word64 and the value to lie in [0, 1].
func TestPoolMarginAsReference(t *testing.T) {
	two64 := new(big.Int).Lsh(big.NewInt(1), 64)
	cases := []struct {
		name string
		rat  *big.Rat
		ok   bool
	}{
		{"zero", big.NewRat(0, 1), true},
		{"one", big.NewRat(1, 1), true},
		{"one half", big.NewRat(1, 2), true},
		{"three halves", big.NewRat(3, 2), false},
		{"negative", big.NewRat(-1, 2), false},
		{
			"2^64 / 2^65 reduces to one half",
			new(big.Rat).SetFrac(two64, new(big.Int).Lsh(two64, 1)),
			true,
		},
		{
			"1 / 2^64",
			new(big.Rat).SetFrac(big.NewInt(1), two64),
			false,
		},
		{"missing", nil, false},
	}
	for _, tc := range cases {
		err := ValidatePoolMargin(GenesisRat{Rat: tc.rat})
		if got := err == nil; got != tc.ok {
			t.Errorf("%s: accepted = %v, reference = %v (err %v)",
				tc.name, got, tc.ok, err)
		}
	}
}

// TestPoolRewardAccountCredentialAsReference checks that the reward
// account's header (CIP-19: 0xe_ key hash, 0xf_ script hash; low nibble the
// network, which cardano-ledger decodes only as 0 or 1) survives decoding,
// clearing the cache to re-encode, and an explicit update.
func TestPoolRewardAccountCredentialAsReference(t *testing.T) {
	for _, tc := range []struct {
		header   string
		credType uint
		network  uint
	}{
		{"e0", CredentialTypeAddrKeyHash, 0},
		{"e1", CredentialTypeAddrKeyHash, 1},
		{"f0", CredentialTypeScriptHash, 0},
		{"f1", CredentialTypeScriptHash, 1},
	} {
		wrapper := decodeRefCertificate(t, refPoolRegistration(
			refHalf, tc.header, "80", "80", "f6",
		))
		cert := wrapper.Certificate.(*PoolRegistrationCertificate)
		check := func(stage string) {
			cred := cert.RewardAccountCredential()
			network, known := cert.RewardAccountNetworkId()
			if cred.CredType != tc.credType || !known ||
				network != tc.network ||
				cred.Credential != CredentialHash(
					bytes.Repeat([]byte{0x33}, 28),
				) {
				t.Fatalf("%s %s: credential %+v network %d known %v",
					tc.header, stage, cred, network, known)
			}
		}
		check("decoded")
		cert.SetCbor(nil)
		check("cache cleared")
		encoded, err := cert.MarshalCBOR()
		if err != nil {
			t.Fatalf("%s: re-encode: %v", tc.header, err)
		}
		wantAccount, _ := hex.DecodeString(
			"581d" + tc.header + refHash(28, 0x33),
		)
		if !bytes.Contains(encoded, wantAccount) {
			t.Fatalf("%s: re-encoding lost reward account header",
				tc.header)
		}
		// Replacing the cache with other bytes drops what was decoded.
		cert.SetCbor(encoded)
		if _, known := cert.RewardAccountNetworkId(); known {
			t.Fatalf("%s: metadata kept after cache replaced", tc.header)
		}
	}
	var cert PoolRegistrationCertificate
	script := Credential{CredType: CredentialTypeScriptHash}
	if err := cert.SetRewardAccountCredential(script, 1); err != nil {
		t.Fatalf("script credential on mainnet: %v", err)
	}
	if got := cert.RewardAccountCredential(); got.CredType !=
		CredentialTypeScriptHash {
		t.Fatalf("credential type %d after update", got.CredType)
	}
	if err := cert.SetRewardAccountCredential(
		Credential{CredType: 2}, 1,
	); err == nil {
		t.Error("credential type 2 accepted")
	}
	if err := cert.SetRewardAccountCredential(script, 2); err == nil {
		t.Error("network 2 accepted")
	}
}
