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

package ledger_test

import (
	"crypto/ed25519"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// TestVerifyOpCertSignatureRejectsIdentityColdKey pins the operational
// certificate boundary, whose reference is the same Ed25519DSIGN as a
// transaction witness.
func TestVerifyOpCertSignatureRejectsIdentityColdKey(t *testing.T) {
	coldVkey := append([]byte{0x01}, make([]byte, 31)...)
	opCert := &ledger.OpCert{
		KesVkey:     make([]byte, 32),
		IssueNumber: 1,
		KesPeriod:   2,
		ColdSignature: append(
			append([]byte{0x01}, make([]byte, 31)...),
			make([]byte, 32)...,
		),
	}
	signable := common.OpCertSignableBytes(
		opCert.KesVkey, opCert.IssueNumber, opCert.KesPeriod,
	)
	if !ed25519.Verify(coldVkey, signable, opCert.ColdSignature) {
		t.Fatal("crypto/ed25519 no longer accepts the identity pair; this test no longer discriminates")
	}
	if err := ledger.VerifyOpCertSignature(opCert, coldVkey); err == nil {
		t.Error("VerifyOpCertSignature accepted an identity cold key and an all-zero signature")
	}
}

func TestVerifyOpCertSignatureAcceptsHonestSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	kesVkey := make([]byte, 32)
	kesVkey[0] = 0x07
	signable := common.OpCertSignableBytes(kesVkey, 3, 4)
	opCert := &ledger.OpCert{
		KesVkey:       kesVkey,
		IssueNumber:   3,
		KesPeriod:     4,
		ColdSignature: ed25519.Sign(priv, signable),
	}
	if err := ledger.VerifyOpCertSignature(opCert, pub); err != nil {
		t.Errorf("rejected an honest opcert signature: %v", err)
	}
	opCert.IssueNumber = 4
	if err := ledger.VerifyOpCertSignature(opCert, pub); err == nil {
		t.Error("accepted an opcert signature after changing the signed body")
	}
}
