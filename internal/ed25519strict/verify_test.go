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

package ed25519strict_test

import (
	"crypto/ed25519"
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/ed25519strict"
)

// IdentityPublicKey and IdentitySignature are the edwards25519 identity point
// and an all-zero scalar. crypto/ed25519.Verify accepts this pair for any
// message; libsodium's crypto_sign_ed25519_verify_detached rejects it.
var (
	identityPublicKey = append([]byte{0x01}, make([]byte, 31)...)
	identitySignature = append(
		append([]byte{0x01}, make([]byte, 31)...),
		make([]byte, 32)...,
	)
)

func TestVerifyRejectsIdentityKey(t *testing.T) {
	for _, msg := range []string{"", "a", "arbitrary transaction body hash"} {
		if !ed25519.Verify(identityPublicKey, []byte(msg), identitySignature) {
			t.Fatalf(
				"crypto/ed25519 no longer accepts the identity pair for %q; this test no longer discriminates",
				msg,
			)
		}
		if ed25519strict.Verify(identityPublicKey, []byte(msg), identitySignature) {
			t.Errorf("strict verification accepted the identity pair for %q", msg)
		}
	}
}

func TestVerifyAcceptsHonestSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("a message this key actually signed")
	sig := ed25519.Sign(priv, msg)
	if !ed25519strict.Verify(pub, msg, sig) {
		t.Error("strict verification rejected an honest signature")
	}
	if ed25519strict.Verify(pub, []byte("a different message"), sig) {
		t.Error("strict verification accepted a signature over a different message")
	}
}

func TestVerifyRejectsMalformedSizes(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("message")
	sig := ed25519.Sign(priv, msg)
	if ed25519strict.Verify(pub[:31], msg, sig) {
		t.Error("accepted a short public key")
	}
	if ed25519strict.Verify(pub, msg, sig[:63]) {
		t.Error("accepted a short signature")
	}
}

// TestVerifyRejectsNonCanonicalScalar covers the third criterion: S must be a
// canonical scalar.
//
// crypto/ed25519 already rejects an unreduced S, so this passes with or without
// the criteria added here -- it documents the boundary rather than guarding it.
// The identity-key cases above are the discriminating ones.
func TestVerifyRejectsNonCanonicalScalar(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("message")
	sig := ed25519.Sign(priv, msg)
	tampered := make([]byte, len(sig))
	copy(tampered, sig)
	tampered[63] |= 0xe0
	if ed25519strict.Verify(pub, msg, tampered) {
		t.Error("accepted a non-canonical S")
	}
}
