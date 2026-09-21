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

package conway_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
)

// previewDRepDepositProposalTxHex is Preview transaction
// 2841a581076167a0662f1b4f1a38bcc8eff386f9ce45c33ae33b1fe8289de210, in block
// ff52e64ffd4a6c8296d4570a40d0e709d4be0f7c32d9deb30417dbe2ecfca519 at absolute
// slot 62103362 (epoch 718). Preview reported protocol major 9 for that epoch
// and did not reach major 10 until epoch 743, so this ParameterChange proposal
// was accepted by the network during the Conway bootstrap phase even though it
// updates DRepDeposit (protocol parameter update key 31).
const previewDRepDepositProposalTxHex = "" +
	"84a800d90102818258209a6f2c1942cac5c8f0742148078350989ec2bffc410fad79e1" +
	"503865d0c3e83e010dd90102818258209a6f2c1942cac5c8f0742148078350989ec2bf" +
	"fc410fad79e1503865d0c3e83e0012d9010281825820f3f61635034140e6cec495a1c6" +
	"9ce85b22690e65ab9553ef408d524f5818364900018182581d6052e63f22c5107ed776" +
	"b70f7b92248b02552fd08f3e747bc745099441821a05ecb4faa2581c34250edd1e9836" +
	"f5378702fbf9416b709bc140e04f668cc355208518a1494154414441636f696e198343" +
	"581ca51445d9fdab07141cfaf5fed2d78732459ba07e266616015cd37d65a14345546b" +
	"1868021a00040081031a03b525480b58206e31d406c021ec5054588b257b978d5b4354" +
	"ce4f9c2f28fc60ff13a81573081814d9010281841b000000174876e800581de0c13582" +
	"aec9a44fcc6d984be003c5058c660e1d2ff1370fd8b49ba73f8400f6a1181f1a0754d4" +
	"c0581cfa24fb305126805cf2164c161d852a0e7330cf988f1fe558cf7d4a6482782968" +
	"747470733a2f2f6d792d69702e61742f746573742f6369703130302d6578616d706c65" +
	"2e6a736f6e58200e93f4447d17fb0d4ca9fd16db19434f0845203711c63274cb6ccfed" +
	"40524251a200d9010281825820742d8af3543349b5b18f3cba28f23b2d6e465b9c136c" +
	"42e1fae6b2390f5654275840edf7f0c05276eaf1272c0b41e62f262eaa02be56254936" +
	"e429eab6be928536b520e622483030f05a0460631bc3c09e5e6257ecb666ad680465ab" +
	"425f30424a0a05a182050082a0821a000950431a07b72dd3f5f6"

// previewDRepDepositProposalSlot is the absolute slot of the block containing
// previewDRepDepositProposalTxHex.
const previewDRepDepositProposalSlot = 62103362

// previewDRepDepositProposalTxId is the id the bytes above hash to. The test
// rests on these being the bytes of a transaction the network accepted, so the
// id is asserted rather than assumed: any other DRep deposit proposal would
// satisfy the field checks while making that claim false.
const previewDRepDepositProposalTxId = "2841a581076167a0662f1b4f1a38bcc8eff386f9ce45c33ae33b1fe8289de210"

// TestBootstrapRulesAcceptCanonicalPreviewDRepDepositProposal pins the absence
// of a field-level bootstrap restriction on ParameterChange proposals against
// the transaction that exposed it, decoded from its own chain bytes rather
// than reconstructed.
//
// The assertion runs over every bootstrap-phase rule descriptor rather than a
// single named rule, so reintroducing a field restriction under any new rule
// id fails here, not only reinstating the removed one.
func TestBootstrapRulesAcceptCanonicalPreviewDRepDepositProposal(t *testing.T) {
	txBytes, err := hex.DecodeString(previewDRepDepositProposalTxHex)
	if err != nil {
		t.Fatalf("decoding transaction hex: %v", err)
	}
	tx, err := conway.NewConwayTransactionFromCbor(txBytes)
	if err != nil {
		t.Fatalf("decoding transaction: %v", err)
	}
	if got := tx.Hash().String(); got != previewDRepDepositProposalTxId {
		t.Fatalf(
			"fixture decodes to transaction %s, want %s",
			got,
			previewDRepDepositProposalTxId,
		)
	}
	proposals := tx.ProposalProcedures()
	if len(proposals) != 1 {
		t.Fatalf("got %d proposal procedures, want 1", len(proposals))
	}
	govAction := proposals[0].GovAction()
	paramChange, ok := govAction.(*conway.ConwayParameterChangeGovAction)
	if !ok {
		t.Fatalf(
			"got governance action %T, want a ParameterChange",
			govAction,
		)
	}
	// Guards against the fixture silently ceasing to exercise the rule: the
	// update must still carry DRepDeposit for this to be a regression test.
	if paramChange.ParamUpdate.DRepDeposit == nil {
		t.Fatal("transaction fixture no longer updates DRepDeposit")
	}
	if got := *paramChange.ParamUpdate.DRepDeposit; got != 123_000_000 {
		t.Fatalf("got DRepDeposit %d, want 123000000", got)
	}

	var bootstrapRules int
	pp := &conway.ConwayProtocolParameters{
		ProtocolVersion: common.ProtocolParametersProtocolVersion{Major: 9},
	}
	for _, descriptor := range conway.UtxoValidationRuleDescriptors() {
		if !strings.HasPrefix(string(descriptor.Id), "bootstrap-") {
			continue
		}
		bootstrapRules++
		if err := descriptor.Validator(
			tx,
			previewDRepDepositProposalSlot,
			nil,
			pp,
		); err != nil {
			t.Errorf("rule %q rejected a canonical transaction: %v",
				descriptor.Id, err)
		}
	}
	if bootstrapRules == 0 {
		t.Fatal("no bootstrap-phase rule descriptors found")
	}
}
