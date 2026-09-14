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
	"crypto/ed25519"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

var (
	identityPublicKey = append([]byte{0x01}, make([]byte, 31)...)
	identitySignature = append(
		append([]byte{0x01}, make([]byte, 31)...),
		make([]byte, 32)...,
	)
)

// TestVerifyVKeySignatureRejectsIdentityKey pins the transaction-witness
// boundary. Cardano.Ledger.Keys.Internal.verifySignedDSIGN reaches libsodium,
// which rejects this pair, so accepting it here is a consensus divergence.
func TestVerifyVKeySignatureRejectsIdentityKey(t *testing.T) {
	msg := []byte("arbitrary transaction body hash")
	if !ed25519.Verify(identityPublicKey, msg, identitySignature) {
		t.Fatal("crypto/ed25519 no longer accepts the identity pair; this test no longer discriminates")
	}
	if err := common.VerifyVKeySignature(
		identityPublicKey, identitySignature, msg,
	); err == nil {
		t.Error("VerifyVKeySignature accepted the identity key and an all-zero signature")
	}
}

func TestVerifyVKeySignatureAcceptsHonestSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("a message this key actually signed")
	sig := ed25519.Sign(priv, msg)
	if err := common.VerifyVKeySignature(pub, sig, msg); err != nil {
		t.Errorf("rejected an honest signature: %v", err)
	}
	if err := common.VerifyVKeySignature(
		pub, sig, []byte("a different message"),
	); err == nil {
		t.Error("accepted a signature over a different message")
	}
}
