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

// A table-driven test of the Shelley certificate lifecycle: stake credential
// registration, delegation and deregistration (DELEG), pool registration,
// re-registration and retirement (POOL), and the epoch boundary that reaps
// retired pools (POOLREAP). The rules below restate
// eras/shelley/formal-spec/delegation.tex and epoch.tex and the reference rules
// in eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Deleg.hs, Pool.hs and
// PoolReap.hs, and Shelley/TxCert.hs for deposits.
//
// gOuroboros keeps no ledger state, so the test plays the caller: it holds the
// reference state, answers LedgerState queries from it, and performs the epoch
// boundary. It walks every state reachable from an empty ledger and, in each,
// every certificate, alone and after each accepted certificate of an earlier
// transaction in the same block (through BlockLedgerState). For each it checks
// that gOuroboros accepts exactly what the reference accepts and charges
// exactly the reference deposit or refund.

package ledger_test

import (
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

const (
	lcKeyDeposit  = 2_000_000
	lcPoolDeposit = 500_000_000
	lcEMax        = 2
	lcMaxEpoch    = 3
	// lcInputAmount is the value of the one input a deposit-checking
	// transaction spends. It is far larger than any deposit, so
	// consumed never equals produced and the validator always reports both.
	lcInputAmount  = 1_000_000_000_000
	lcOutputAmount = 1
	lcAddress      = "addr_test1qqx80sj9nwxdnglmzdl95v2k40d9422au0klwav8jz2dj985v0wma0mza32f8z6pv2jmkn7cen50f9vn9jmp7dd0njcqqpce07"
	lcCreds        = 2
	// lcRewardCred is the credential the pool's reward account belongs to,
	// which a reaped pool's deposit is refunded to.
	lcRewardCred = 0
)

var (
	lcPool = poolKeyHash(0x61)
	lcVrf  = vrfKeyHash(0x62)
)

// lcState is the ledger as the reference holds it. A balance is zero or
// non-zero; the amount does not change any rule.
type lcState struct {
	epoch      uint64
	registered [lcCreds]bool
	funded     [lcCreds]bool
	delegated  [lcCreds]bool
	poolReg    bool
	retiring   uint64 // 0 for none
}

type lcKind int

const (
	lcRegKey lcKind = iota
	lcDeregKey
	lcDeleg
	lcRegPool
	lcRetirePool
)

// lcCert is one certificate. cred is used by the stake kinds; epochOffset
// by a retirement, whose epoch is the current epoch plus it.
type lcCert struct {
	kind        lcKind
	cred        int
	epochOffset uint64
}

func (c lcCert) String() string {
	switch c.kind {
	case lcRegKey:
		return fmt.Sprintf("RegKey(c%d)", c.cred)
	case lcDeregKey:
		return fmt.Sprintf("DeregKey(c%d)", c.cred)
	case lcDeleg:
		return fmt.Sprintf("Deleg(c%d)", c.cred)
	case lcRegPool:
		return "RegPool"
	default:
		return fmt.Sprintf("RetirePool(+%d)", c.epochOffset)
	}
}

func lcAllCerts() []lcCert {
	var certs []lcCert
	for i := range lcCreds {
		certs = append(certs,
			lcCert{kind: lcRegKey, cred: i},
			lcCert{kind: lcDeregKey, cred: i},
			lcCert{kind: lcDeleg, cred: i},
		)
	}
	certs = append(certs, lcCert{kind: lcRegPool})
	// Offsets 0 and eMax+1 are the two bounds of Pool-Retire
	// (cepoch < e <= cepoch + emax).
	for off := range uint64(lcEMax + 2) {
		certs = append(certs, lcCert{kind: lcRetirePool, epochOffset: off})
	}
	return certs
}

// accepts is the reference acceptance (Deleg.hs delegationTransition, Pool.hs
// poolTransition): Deleg-Reg, Deleg-Dereg (empty account only), Deleg-Deleg
// with DELEGS' registered-pool check, Pool-Reg/reReg, and Pool-Retire.
func (s lcState) accepts(c lcCert) bool {
	switch c.kind {
	case lcRegKey:
		return !s.registered[c.cred]
	case lcDeregKey:
		return s.registered[c.cred] && !s.funded[c.cred]
	case lcDeleg:
		return s.registered[c.cred] && s.poolReg
	case lcRegPool:
		return true
	default:
		e := s.epoch + c.epochOffset
		return s.poolReg && s.epoch < e && e <= s.epoch+lcEMax
	}
}

// deposit is the reference deposit change: what the certificate adds to the
// deposit pot, negative for a refund. A pool deposit is due only for a pool
// not already registered (shelleyTotalDepositsTxCerts).
func (s lcState) deposit(c lcCert) int64 {
	switch c.kind {
	case lcRegKey:
		return lcKeyDeposit
	case lcDeregKey:
		return -lcKeyDeposit
	case lcRegPool:
		if !s.poolReg {
			return lcPoolDeposit
		}
		return 0
	default:
		return 0
	}
}

// apply is the reference effect of an accepted certificate. A
// re-registration cancels a pending retirement; its new parameters wait
// for the boundary and change no rule here.
func (s lcState) apply(c lcCert) lcState {
	switch c.kind {
	case lcRegKey:
		s.registered[c.cred] = true
		s.funded[c.cred] = false
	case lcDeregKey:
		s.registered[c.cred] = false
		s.delegated[c.cred] = false
	case lcDeleg:
		s.delegated[c.cred] = true
	case lcRegPool:
		s.poolReg = true
		s.retiring = 0
	case lcRetirePool:
		s.retiring = s.epoch + c.epochOffset
	}
	return s
}

// boundary is the epoch boundary's POOLREAP: a pool retiring in the new
// epoch leaves the pool set, its deposit goes to its reward account if that
// is registered, and delegations to it are cleared.
func (s lcState) boundary() lcState {
	s.epoch++
	if s.poolReg && s.retiring == s.epoch {
		s.poolReg = false
		s.retiring = 0
		s.delegated = [lcCreds]bool{}
		if s.registered[lcRewardCred] {
			s.funded[lcRewardCred] = true
		}
	}
	return s
}

// successors are the states one step away: an accepted certificate, a
// reward reaching or a full withdrawal from a registered account, or the
// epoch boundary.
func (s lcState) successors() []lcState {
	var next []lcState
	for _, c := range lcAllCerts() {
		if s.accepts(c) {
			next = append(next, s.apply(c))
		}
	}
	for i := range lcCreds {
		if s.registered[i] {
			t := s
			t.funded[i] = !s.funded[i]
			next = append(next, t)
		}
	}
	if s.epoch < lcMaxEpoch {
		next = append(next, s.boundary())
	}
	return next
}

func lcCredential(i int) common.Credential {
	return stakeKeyCredential(0x70 + byte(i))
}

func lcCredIndex(cred common.Credential) (int, bool) {
	for i := range lcCreds {
		if want := lcCredential(i); want.CredType == cred.CredType &&
			want.Credential == cred.Credential {
			return i, true
		}
	}
	return 0, false
}

// lcView answers the caller's LedgerState queries from a test state, as a
// caller that commits every block and reaps at the boundary would.
type lcView struct {
	common.LedgerState
	s lcState
}

func (v lcView) IsStakeCredentialRegistered(cred common.Credential) bool {
	i, ok := lcCredIndex(cred)
	return ok && v.s.registered[i]
}

func (v lcView) IsRewardAccountRegistered(cred common.Credential) bool {
	return v.IsStakeCredentialRegistered(cred)
}

func (v lcView) RewardAccountBalance(
	cred common.Credential,
) (*uint64, error) {
	i, ok := lcCredIndex(cred)
	if !ok || !v.s.registered[i] {
		return nil, nil
	}
	balance := uint64(0)
	if v.s.funded[i] {
		balance = 1
	}
	return &balance, nil
}

func (v lcView) StakeCredentialDeposit(
	cred common.Credential,
) (*uint64, error) {
	if !v.IsStakeCredentialRegistered(cred) {
		return nil, nil
	}
	deposit := uint64(lcKeyDeposit)
	return &deposit, nil
}

func (v lcView) PoolCurrentState(
	pool common.PoolKeyHash,
) (*common.PoolRegistrationCertificate, *uint64, error) {
	if pool != lcPool || !v.s.poolReg {
		return nil, nil, nil
	}
	if v.s.retiring == 0 {
		return lcPoolRegistration(), nil, nil
	}
	retiring := v.s.retiring
	return lcPoolRegistration(), &retiring, nil
}

func (v lcView) IsPoolRegistered(pool common.PoolKeyHash) bool {
	return pool == lcPool && v.s.poolReg
}

func (v lcView) IsVrfKeyInUse(
	vrf common.Blake2b256,
) (bool, common.PoolKeyHash, error) {
	if v.s.poolReg && vrf == lcVrf {
		return true, lcPool, nil
	}
	return false, common.PoolKeyHash{}, nil
}

func (v lcView) EpochForSlot(uint64) (uint64, error) {
	return v.s.epoch, nil
}

func (v lcView) UtxoById(input common.TransactionInput) (common.Utxo, error) {
	output, err := mockledger.NewSimpleTransactionOutput(
		lcAddress,
		lcInputAmount,
	)
	if err != nil {
		return common.Utxo{}, err
	}
	return common.Utxo{Id: input, Output: output}, nil
}

var (
	_ common.EpochState                  = lcView{}
	_ common.StakeCredentialDepositState = lcView{}
)

func lcPoolRegistration() *common.PoolRegistrationCertificate {
	return &common.PoolRegistrationCertificate{
		CertType:   uint(common.CertificateTypePoolRegistration),
		Operator:   lcPool,
		VrfKeyHash: lcVrf,
		Pledge:     1,
		Cost:       0,
		Margin:     common.NewGenesisRat(0, 1),
	}
}

func lcCertificate(s lcState, c lcCert) common.Certificate {
	switch c.kind {
	case lcRegKey:
		return &common.StakeRegistrationCertificate{
			CertType:        uint(common.CertificateTypeStakeRegistration),
			StakeCredential: lcCredential(c.cred),
		}
	case lcDeregKey:
		return &common.StakeDeregistrationCertificate{
			CertType:        uint(common.CertificateTypeStakeDeregistration),
			StakeCredential: lcCredential(c.cred),
		}
	case lcDeleg:
		cred := lcCredential(c.cred)
		return &common.StakeDelegationCertificate{
			CertType:        uint(common.CertificateTypeStakeDelegation),
			StakeCredential: &cred,
			PoolKeyHash:     lcPool,
		}
	case lcRegPool:
		return lcPoolRegistration()
	default:
		return poolRetirementCert(lcPool, s.epoch+c.epochOffset)
	}
}

// lcTx builds a transaction carrying one certificate, spending one input of
// lcInputAmount (lcView answers for any input) and paying one output of
// lcOutputAmount with no fee, so the value conservation check reports the
// deposit. The input's transaction id differs from every transaction's own
// id, so a transaction applied earlier in the block never spends it.
func lcTx(
	t *testing.T,
	id byte,
	cert common.Certificate,
) *mockledger.MockTransaction {
	t.Helper()
	txId := make([]byte, 32)
	txId[0] = id
	inputId := make([]byte, 32)
	inputId[0] = id
	inputId[1] = 0xff
	input, err := mockledger.NewSimpleTransactionInput(inputId, 0)
	require.NoError(t, err)
	output, err := mockledger.NewSimpleTransactionOutput(
		lcAddress,
		lcOutputAmount,
	)
	require.NoError(t, err)
	tx, err := mockledger.NewTransactionBuilder().
		WithId(txId).
		WithInputs(input).
		WithOutputs(output).
		WithValid(true).
		Build()
	require.NoError(t, err)
	return tx.(*mockledger.MockTransaction).WithCertificates(cert)
}

func lcPparams() *shelley.ShelleyProtocolParameters {
	pparams := mockledger.NewMockShelleyProtocolParams()
	pparams.ProtocolMajor = 2
	pparams.KeyDeposit = lcKeyDeposit
	pparams.PoolDeposit = lcPoolDeposit
	pparams.MaxEpoch = lcEMax
	pparams.MinPoolCost = 0
	return &pparams
}

// lcGoVerdict runs the DELEG and POOL rules gOuroboros registers for
// Shelley and, when both accept, the value conservation rule. It returns
// whether the certificate was accepted, the rejection, and the deposit
// gOuroboros charged: produced minus consumed, less the output and plus
// the input.
func lcGoVerdict(
	t *testing.T,
	tx common.Transaction,
	ls common.LedgerState,
	pp *shelley.ShelleyProtocolParameters,
) (bool, error, int64) {
	t.Helper()
	if err := shelley.UtxoValidateDelegation(tx, 0, ls, pp); err != nil {
		return false, err, 0
	}
	if err := shelley.UtxoValidatePoolCertificates(tx, 0, ls, pp); err != nil {
		return false, err, 0
	}
	err := shelley.UtxoValidateValueNotConservedUtxo(tx, 0, ls, pp)
	var mismatch shelley.ValueNotConservedUtxoError
	require.True(
		t,
		errors.As(err, &mismatch),
		"value conservation did not report consumed and produced: %v",
		err,
	)
	net := new(big.Int).Sub(mismatch.Produced, mismatch.Consumed)
	net.Add(net, big.NewInt(lcInputAmount-lcOutputAmount))
	require.True(t, net.IsInt64())
	return true, nil, net.Int64()
}

// lcCheck compares gOuroboros with the reference for every certificate in state
// s, read through ls. It returns how many cells it checked.
func lcCheck(
	t *testing.T,
	label string,
	s lcState,
	ls common.LedgerState,
	pp *shelley.ShelleyProtocolParameters,
) int {
	t.Helper()
	cells := 0
	for _, c := range lcAllCerts() {
		tx := lcTx(t, 0xf0, lcCertificate(s, c))
		accepted, err, deposit := lcGoVerdict(t, tx, ls, pp)
		want := s.accepts(c)
		require.Equal(
			t, want, accepted,
			"%s: %s in %+v: reference accepts=%v, gOuroboros err=%v",
			label, c, s, want, err,
		)
		if want {
			require.Equal(
				t, s.deposit(c), deposit,
				"%s: %s in %+v: deposit", label, c, s,
			)
		}
		cells++
	}
	return cells
}

// TestCertLifecycleModel walks every state reachable from an empty ledger
// within lcMaxEpoch epochs. In each state it checks every certificate against a
// fresh block, and again after each accepted certificate of an earlier
// transaction in the block, which BlockLedgerState must fold in.
func TestCertLifecycleModel(t *testing.T) {
	t.Parallel()
	pp := lcPparams()
	base := mockledger.NewLedgerStateBuilder().Build()

	start := lcState{}
	seen := map[lcState]bool{start: true}
	queue := []lcState{start}
	var cells, blocks int
	reachedReaped, reachedRetiring, reachedFunded := false, false, false
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		reachedRetiring = reachedRetiring || s.retiring != 0
		reachedFunded = reachedFunded || s.funded[lcRewardCred]
		reachedReaped = reachedReaped || (!s.poolReg && s.epoch > 0 &&
			s.funded[lcRewardCred] && s.registered[lcRewardCred])

		cells += lcCheck(t, "fresh block", s, lcView{base, s}, pp)
		for i, prior := range lcAllCerts() {
			if !s.accepts(prior) {
				continue
			}
			block := common.NewBlockLedgerState(lcView{base, s})
			require.NoError(t, block.ApplyTransaction(
				lcTx(t, byte(i), lcCertificate(s, prior)),
				pp,
			))
			label := "after " + prior.String()
			cells += lcCheck(t, label, s.apply(prior), block, pp)
			blocks++
		}

		for _, next := range s.successors() {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	// These guard against a walk that silently stops early: each lifecycle
	// stage must have been reached.
	require.True(t, reachedRetiring, "no state with a pending retirement")
	require.True(t, reachedFunded, "no state with a funded reward account")
	require.True(t, reachedReaped, "no state after a pool was reaped")
	t.Logf(
		"%d states, %d two-transaction blocks, %d checked cells",
		len(seen), blocks, cells,
	)
}
