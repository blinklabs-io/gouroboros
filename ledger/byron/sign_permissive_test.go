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

package byron

import (
	"crypto/ed25519"
	"testing"
)

// TestVerifyEd25519StaysPermissive is the guard against a well-meaning sweep.
//
// Byron's reference is Cardano.Crypto.Signing.Signature.verifySignatureRaw ->
// CC.verify in cardano-crypto-wallet, whose bundled ed25519-donna
// ed25519_sign_open checks only the high bits of S and performs no small-order
// test. It accepts the edwards25519 identity pair. Byron blocks are immutable,
// so tightening this boundary to match the non-Byron ones would reject chain
// the node accepts and break sync from genesis.
func TestVerifyEd25519StaysPermissive(t *testing.T) {
	// A Byron extended verification key is the 32-byte Ed25519 key followed by
	// a 32-byte chain code; only the first half is verified against.
	verificationKey := make([]byte, VerificationKeySize)
	verificationKey[0] = 0x01
	signature := append(
		append([]byte{0x01}, make([]byte, 31)...),
		make([]byte, 32)...,
	)
	if !verifyEd25519(
		verificationKey,
		[]byte("any Byron payload"),
		signature,
	) {
		t.Error(
			"Byron verification rejected the identity pair; ed25519-donna accepts it, " +
				"so this boundary must not use the strict criteria",
		)
	}
}

func TestVerifyEd25519AcceptsHonestSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	verificationKey := make([]byte, VerificationKeySize)
	copy(verificationKey, pub)
	msg := []byte("a Byron payload this key actually signed")
	sig := ed25519.Sign(priv, msg)
	if !verifyEd25519(verificationKey, msg, sig) {
		t.Error("rejected an honest Byron signature")
	}
	if verifyEd25519(verificationKey, []byte("a different payload"), sig) {
		t.Error("accepted a Byron signature over a different payload")
	}
}
