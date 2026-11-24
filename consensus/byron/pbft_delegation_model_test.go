// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package byron

import (
	"crypto/ed25519"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// The constants of ByronDelegation.cfg: two genesis keys, g1 starting
// delegated to the outsider d1, k = 1, three slots per epoch, blocks up to
// slot 4.
const (
	delegationModelK             = uint64(1)
	delegationModelSlotsPerEpoch = uint64(3)
	delegationModelMaxSlot       = uint64(4)
	delegationModelMagic         = uint32(42)
)

type delegationModelCert struct {
	dlg, del string
	ep       uint64
}

type delegationModelSched struct {
	at       uint64
	dlg, del string
}

// delegationModelRef transcribes the reference delegation interface that
// ByronDelegation.tla runs as its r* variables (cardano-ledger 1210d37f,
// Cardano/Chain/Delegation/Validation/Interface.hs updateDelegation,
// Scheduling.hs scheduleCertificate, Activation.hs activateDelegation).
type delegationModelRef struct {
	sds []delegationModelSched
	eks map[[2]string]bool
	dms map[string]string
	dws map[string]uint64
}

func (r delegationModelRef) clone() delegationModelRef {
	next := delegationModelRef{
		sds: slices.Clone(r.sds),
		eks: map[[2]string]bool{},
		dms: map[string]string{},
		dws: map[string]uint64{},
	}
	for k, v := range r.eks {
		next.eks[k] = v
	}
	for k, v := range r.dms {
		next.dms[k] = v
	}
	for k, v := range r.dws {
		next.dws[k] = v
	}
	return next
}

func (r delegationModelRef) key(slot uint64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%v|", slot, r.sds)
	var eks []string
	for k := range r.eks {
		eks = append(eks, k[0]+"/"+k[1])
	}
	sort.Strings(eks)
	fmt.Fprintf(&b, "%v|%v|%v", eks, r.dms, r.dws)
	return b.String()
}

// apply is updateDelegation: schedule every certificate (any failure
// rejects the block), then tick.
func (r delegationModelRef) apply(
	genesis map[string]bool,
	epoch, slot uint64,
	payload []delegationModelCert,
) (delegationModelRef, bool) {
	next := r.clone()
	act := slot + 2*delegationModelK
	for _, c := range payload {
		if !genesis[c.dlg] ||
			c.ep < epoch || c.ep > epoch+1 ||
			next.eks[[2]string{fmt.Sprint(c.ep), c.dlg}] {
			return r, false
		}
		for _, s := range next.sds {
			if s.at == act && s.dlg == c.dlg {
				return r, false
			}
		}
		next.sds = append(next.sds, delegationModelSched{act, c.dlg, c.del})
		next.eks[[2]string{fmt.Sprint(c.ep), c.dlg}] = true
	}
	for _, s := range next.sds {
		if s.at > slot {
			continue
		}
		inRange := false
		for _, d := range next.dms {
			if d == s.del {
				inRange = true
			}
		}
		if !inRange && (next.dws[s.dlg] < s.at || s.at == 0) {
			next.dms[s.dlg] = s.del
			next.dws[s.dlg] = s.at
		}
	}
	kept := next.sds[:0]
	for _, s := range next.sds {
		if s.at >= slot+1 {
			kept = append(kept, s)
		}
	}
	next.sds = kept
	for k := range next.eks {
		var ep uint64
		fmt.Sscan(k[0], &ep)
		if ep < epoch {
			delete(next.eks, k)
		}
	}
	return next, true
}

// TestPBFTDelegationModelTable walks every chain of ByronDelegation.cfg: a
// block one or two slots after the last, carrying no certificate, one, or
// (in the first block) two, from any of g1, g2 or the non-genesis x, to
// any of g1, g2 or d1, for the previous, current, next or second-next
// epoch. On every block gOuroboros must accept exactly the payloads the
// reference accepts (VerdictsAgree) and end with the same active map
// (MapsAgree), schedule and key-epoch set.
//
// The activation slots are not compared: on a re-delegation to the
// delegate already active the reference does nothing and gOuroboros
// rewrites the slot (ProbeSlotsDiffer). The test counts that it happens
// and that the maps still agree.
func TestPBFTDelegationModelTable(t *testing.T) {
	keys := map[string][]byte{}
	privs := map[string]ed25519.PrivateKey{}
	hashes := map[string]common.Blake2b224{}
	names := map[common.Blake2b224]string{}
	for i, name := range []string{"g1", "g2", "x", "d1"} {
		vk, sk := deterministicPBFTVerificationKey(byte(0x71 + i))
		hash, err := PBFTVerificationKeyHash(vk)
		require.NoError(t, err)
		keys[name], privs[name], hashes[name], names[hash] = vk, sk, hash, name
	}
	genesis := map[string]bool{"g1": true, "g2": true}

	initial, err := NewPBFTDelegationState(ByronConfig{
		ProtocolMagic:    delegationModelMagic,
		SecurityParam:    delegationModelK,
		GenesisKeyHashes: [][]byte{hashes["g1"].Bytes(), hashes["g2"].Bytes()},
		GenesisDelegations: map[common.Blake2b224]common.Blake2b224{
			hashes["g1"]: hashes["d1"],
		},
		GenesisDelegationEpochs: map[common.Blake2b224]uint64{hashes["g1"]: 0},
	})
	require.NoError(t, err)
	ref := delegationModelRef{
		eks: map[[2]string]bool{{"0", "g1"}: true},
		dms: map[string]string{"g1": "d1", "g2": "g2"},
		dws: map[string]uint64{"g1": 0, "g2": 0},
	}

	signed := map[delegationModelCert]cbor.RawMessage{}
	sign := func(c delegationModelCert) cbor.RawMessage {
		if raw, ok := signed[c]; ok {
			return raw
		}
		raw := signedPBFTDelegationCertificate(
			t, delegationModelMagic, c.ep, keys[c.dlg], privs[c.dlg], keys[c.del],
		)
		signed[c] = raw
		return raw
	}
	certsFor := func(epoch uint64) []delegationModelCert {
		var certs []delegationModelCert
		for _, dlg := range []string{"g1", "g2", "x"} {
			for _, del := range []string{"g1", "g2", "d1"} {
				for ep := epoch; ep <= epoch+2; ep++ {
					certs = append(certs, delegationModelCert{dlg, del, ep})
				}
				if epoch > 0 {
					certs = append(certs, delegationModelCert{dlg, del, epoch - 1})
				}
			}
		}
		return certs
	}

	type node struct {
		slot uint64
		ref  delegationModelRef
		go_  PBFTDelegationState
	}
	queue := []node{{0, ref, initial}}
	seen := map[string]bool{ref.key(0): true}
	counts := map[string]int{}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for slot := n.slot + 1; slot <= n.slot+2 && slot <= delegationModelMaxSlot; slot++ {
			epoch := slot / delegationModelSlotsPerEpoch
			certs := certsFor(epoch)
			payloads := [][]delegationModelCert{nil}
			for _, c := range certs {
				payloads = append(payloads, []delegationModelCert{c})
			}
			if n.slot == 0 {
				for _, c := range certs {
					for _, d := range certs {
						payloads = append(payloads, []delegationModelCert{c, d})
					}
				}
			}
			for _, payload := range payloads {
				raw := make([]cbor.RawMessage, 0, len(payload))
				for _, c := range payload {
					raw = append(raw, sign(c))
				}
				wantRef, refOK := n.ref.apply(genesis, epoch, slot, payload)
				gotGo, err := n.go_.ApplyPayloadCbor(epoch, slot, raw)
				name := fmt.Sprintf("from %s at slot %d: %v", n.ref.key(n.slot), slot, payload)
				require.Equal(t, refOK, err == nil, "VerdictsAgree %s: %v", name, err)
				if !refOK {
					counts["rejected"]++
					continue
				}
				counts["accepted"]++

				active := map[string]string{}
				for dlg, del := range gotGo.ActiveDelegations() {
					active[names[dlg]] = names[del]
				}
				require.Equal(t, wantRef.dms, active, "MapsAgree %s", name)
				var scheduled []delegationModelSched
				for _, s := range gotGo.scheduledDelegations {
					scheduled = append(scheduled, delegationModelSched{
						s.activationSlot, names[s.delegator], names[s.delegate],
					})
				}
				require.Equal(t, len(wantRef.sds), len(scheduled), "schedule %s", name)
				for i := range scheduled {
					require.Equal(t, wantRef.sds[i], scheduled[i], "schedule %s", name)
				}
				keyEpochs := map[[2]string]bool{}
				for ke := range gotGo.keyEpochDelegations {
					keyEpochs[[2]string{fmt.Sprint(ke.epoch), names[ke.delegator]}] = true
				}
				require.Equal(t, wantRef.eks, keyEpochs, "key epochs %s", name)
				for g := range genesis {
					if gotGo.delegationSlots[hashes[g]] != wantRef.dws[g] {
						counts["activation slots differ"]++
					}
				}
				if wantRef.dms["g1"] == "g1" {
					counts["revoked to self"]++
				}
				if wantRef.dms["g2"] == "d1" {
					counts["delegate moved"]++
				}

				key := wantRef.key(slot)
				if !seen[key] {
					seen[key] = true
					queue = append(queue, node{slot, wantRef, gotGo})
				}
			}
		}
	}
	for _, group := range []string{
		"accepted",
		"rejected",
		"revoked to self",
		"delegate moved",
		"activation slots differ",
	} {
		require.Positive(t, counts[group], group)
	}
	t.Logf("%d states, %v", len(seen), counts)
}
