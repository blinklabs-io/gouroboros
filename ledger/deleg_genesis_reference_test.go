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
	"errors"
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// refGenesisState is a transcription of the genesis delegation part of the
// reference DState: genDelegs, keyed by genesis key, and futureGenDelegs,
// keyed by (activation slot, genesis key). Source: the GenesisDelegTxCert
// case of delegationTransition, cardano-ledger 1210d37f
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Deleg.hs 343-369.
type refGenesisState struct {
	current map[byte][2]byte
	future  map[[2]byte][2]byte
}

func (s refGenesisState) clone() refGenesisState {
	c := refGenesisState{
		current: map[byte][2]byte{},
		future:  map[[2]byte][2]byte{},
	}
	for k, v := range s.current {
		c.current[k] = v
	}
	for k, v := range s.future {
		c.future[k] = v
	}
	return c
}

// apply returns the reference verdict for GenesisDelegTxCert gkh vkh vrf at
// activation slot slot, and the state after it when it is accepted.
func (s refGenesisState) apply(slot, gkh, vkh, vrf byte) string {
	if _, ok := s.current[gkh]; !ok {
		return "GenesisKeyNotInMappingDELEG"
	}
	coldKeys := map[byte]bool{}
	vrfKeys := map[byte]bool{}
	for g, pair := range s.current {
		if g != gkh {
			coldKeys[pair[0]], vrfKeys[pair[1]] = true, true
		}
	}
	for key, pair := range s.future {
		if key[1] != gkh {
			coldKeys[pair[0]], vrfKeys[pair[1]] = true, true
		}
	}
	if coldKeys[vkh] {
		return "DuplicateGenesisDelegateDELEG"
	}
	if vrfKeys[vrf] {
		return "DuplicateGenesisVRFDELEG"
	}
	s.future[[2]byte{slot, gkh}] = [2]byte{vkh, vrf}
	return ""
}

func goGenesisVerdict(err error) string {
	var notIn shelley.GenesisKeyNotInMappingError
	var dupDelegate shelley.DuplicateGenesisDelegateError
	var dupVrf shelley.DuplicateGenesisVRFError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &notIn):
		return "GenesisKeyNotInMappingDELEG"
	case errors.As(err, &dupDelegate):
		return "DuplicateGenesisDelegateDELEG"
	case errors.As(err, &dupVrf):
		return "DuplicateGenesisVRFDELEG"
	}
	return "other: " + err.Error()
}

// TestGenesisDelegationSequencesAsReference drives every block of one to
// three single-certificate transactions, and every transaction of two
// certificates, over three genesis keys (one outside the mapping), four
// delegates and four VRF keys, through the Shelley delegation rule and
// BlockLedgerState (GenesisDelegationFromCertificate, GenesisDelegationsFor,
// ScheduleGenesisDelegation), and checks each verdict against the
// transcription above. The base state is newGenesisRegistry's: genesis keys
// 0x01 and 0x02 in force and a delegation of 0x01 scheduled in an earlier
// slot.
func TestGenesisDelegationSequencesAsReference(t *testing.T) {
	const earlierSlot, blockSlot = 0, 1
	base := refGenesisState{
		current: map[byte][2]byte{0x01: {0x11, 0x21}, 0x02: {0x12, 0x22}},
		future:  map[[2]byte][2]byte{{earlierSlot, 0x01}: {0x13, 0x23}},
	}
	type cert struct{ g, d, v byte }
	var certs []cert
	for _, g := range []byte{0x01, 0x02, 0x03} {
		for _, d := range []byte{0x11, 0x12, 0x13, 0x31} {
			for _, v := range []byte{0x21, 0x22, 0x23, 0x41} {
				certs = append(certs, cert{g, d, v})
			}
		}
	}
	pparams := maryPparams(0)
	verdicts := map[string]int{}
	check := func(seq []cert) {
		ref := base.clone()
		block := common.NewBlockLedgerState(newGenesisRegistry())
		for i, c := range seq {
			want := ref.apply(blockSlot, c.g, c.d, c.v)
			tx := poolCertTx(genesisDelegCert(c.g, c.d, c.v))
			got := goGenesisVerdict(
				shelley.UtxoValidateDelegation(tx, 0, block, pparams),
			)
			verdicts[want]++
			if got != want {
				t.Fatalf("%v step %d: got %q, reference %q", seq, i, got, want)
			}
			if got != "" {
				return
			}
			if err := block.ApplyTransaction(tx, pparams); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, a := range certs {
		check([]cert{a})
		for _, b := range certs {
			check([]cert{a, b})
			for _, c := range certs {
				check([]cert{a, b, c})
			}
		}
	}
	// Two certificates in one transaction: the rule threads the state
	// from the first to the second and the first failure is the verdict.
	for _, a := range certs {
		for _, b := range certs {
			ref := base.clone()
			want := ref.apply(blockSlot, a.g, a.d, a.v)
			if want == "" {
				want = ref.apply(blockSlot, b.g, b.d, b.v)
			}
			got := goGenesisVerdict(shelley.UtxoValidateDelegation(
				poolCertTx(
					genesisDelegCert(a.g, a.d, a.v),
					genesisDelegCert(b.g, b.d, b.v),
				),
				0,
				newGenesisRegistry(),
				pparams,
			))
			if got != want {
				t.Fatalf("tx %v %v: got %q, reference %q", a, b, got, want)
			}
		}
	}
	for _, kind := range []string{
		"", "GenesisKeyNotInMappingDELEG", "DuplicateGenesisDelegateDELEG",
		"DuplicateGenesisVRFDELEG",
	} {
		if verdicts[kind] == 0 {
			t.Errorf("no step reached verdict %q", kind)
		}
	}
	t.Log(fmt.Sprint(verdicts))
}

// TestGenesisDelegationFromCertificateAsCDDL follows shelley.cddl
// genesis_key_delegation = (5, genesis_hash, genesis_delegate_hash,
// vrf_keyhash), with 28-byte genesis and delegate hashes: a hash of any
// other length is refused.
func TestGenesisDelegationFromCertificateAsCDDL(t *testing.T) {
	good := make([]byte, 28)
	for _, c := range []struct {
		genesis, delegate []byte
		ok                bool
	}{
		{good, good, true},
		{good[:27], good, false},
		{append(good, 0), good, false},
		{good, good[:27], false},
		{good, append(good, 0), false},
		{nil, good, false},
	} {
		_, err := common.GenesisDelegationFromCertificate(
			&common.GenesisKeyDelegationCertificate{
				GenesisHash:         c.genesis,
				GenesisDelegateHash: c.delegate,
			},
		)
		if (err == nil) != c.ok {
			t.Errorf(
				"genesis %d bytes, delegate %d bytes: err %v",
				len(c.genesis), len(c.delegate), err,
			)
		}
	}
	d, err := common.GenesisDelegationFromCertificate(
		genesisDelegCert(0x05, 0x06, 0x07).(*common.GenesisKeyDelegationCertificate),
	)
	if err != nil || d != genesisDelegation(0x05, 0x06, 0x07) {
		t.Errorf("got %+v, %v", d, err)
	}
	scheduled := common.ScheduleGenesisDelegation(nil, d)
	scheduled = common.ScheduleGenesisDelegation(
		scheduled, genesisDelegation(0x08, 0x09, 0x0a),
	)
	scheduled = common.ScheduleGenesisDelegation(
		scheduled, genesisDelegation(0x05, 0x0b, 0x0c),
	)
	if len(scheduled) != 2 ||
		scheduled[0] != genesisDelegation(0x05, 0x0b, 0x0c) ||
		scheduled[1] != genesisDelegation(0x08, 0x09, 0x0a) {
		t.Errorf("scheduled %+v", scheduled)
	}
}
