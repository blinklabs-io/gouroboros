// Copyright 2026 Blink Labs Software

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

// Related files:
//   - state.go: LedgerState interface used by validation rules
//   - tx.go: Transaction interface that rules validate
//   - errors.go: Common error types returned by rules
//   - ledger/shelley/rules.go: Base validation rules (other eras delegate here)
//   - ledger/{era}/rules.go: Era-specific validation rules

import (
	"bytes"
	"fmt"
	"math/big"
	"math/bits"
	"reflect"
	"sort"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// UtxoValidationRuleFunc represents a function that validates a transaction
// against a specific UTXO validation rule.
type UtxoValidationRuleFunc func(
	tx Transaction,
	slot uint64,
	ledgerState LedgerState,
	protocolParams ProtocolParameters,
) error

type cachedUtxoLookup struct {
	utxo Utxo
	err  error
}

// cachedLedgerState keeps read-only UTxO lookups transaction-scoped. Several
// validation rules need the same transaction view; sharing these results
// avoids resolving each input again as the rule list advances.
type cachedLedgerState struct {
	LedgerState
	lookups map[string]cachedUtxoLookup
}

func (s *cachedLedgerState) UnderlyingLedgerState() LedgerState {
	return s.LedgerState
}

// UnwrapLedgerState returns the caller's ledger state when validation is
// running with the transaction-scoped UTxO lookup cache. Rules that inspect
// optional LedgerState capabilities should use this before type assertions.
func UnwrapLedgerState(ledgerState LedgerState) LedgerState {
	if cached, ok := ledgerState.(interface {
		UnderlyingLedgerState() LedgerState
	}); ok {
		return cached.UnderlyingLedgerState()
	}
	return ledgerState
}

func (s *cachedLedgerState) UtxoById(input TransactionInput) (Utxo, error) {
	key := input.String()
	if result, ok := s.lookups[key]; ok {
		return result.utxo, result.err
	}
	utxo, err := s.LedgerState.UtxoById(input)
	s.lookups[key] = cachedUtxoLookup{utxo: utxo, err: err}
	return utxo, err
}

// UtxoValidateCurrentTreasuryValue checks a transaction's optional current
// treasury value against the ledger state.
func UtxoValidateCurrentTreasuryValue(
	tx Transaction, slot uint64, ledgerState LedgerState, protocolParams ProtocolParameters,
) error {
	if !tx.IsValid() {
		return nil
	}
	bodies := SubTransactionBodiesFromTransaction(tx)
	values := make([]*big.Int, 0, len(bodies)+1)
	for _, body := range bodies {
		if body != nil && TransactionCurrentTreasuryValuePresent(body) {
			values = append(values, body.CurrentTreasuryValue())
		}
	}
	if TransactionCurrentTreasuryValuePresent(tx) {
		values = append(values, tx.CurrentTreasuryValue())
	}
	if len(values) == 0 {
		return nil
	}
	if ledgerState == nil || (reflect.ValueOf(ledgerState).Kind() == reflect.Pointer && reflect.ValueOf(ledgerState).IsNil()) {
		return TreasuryValueQueryError{Err: TreasuryValueProviderUnavailableError{}}
	}
	expected, err := ledgerState.TreasuryValue()
	if err != nil {
		return TreasuryValueQueryError{Err: err}
	}
	for _, supplied := range values {
		if supplied.Cmp(new(big.Int).SetUint64(expected)) != 0 {
			return CurrentTreasuryValueMismatchError{Supplied: new(big.Int).Set(supplied), Expected: expected}
		}
	}
	return nil
}

// UtxoValidationRuleGroup describes a consecutive group of transaction
// validation rules with the same phase-2 validity scope. Construct groups with
// AlwaysUtxoValidationRules or Phase2ValidUtxoValidationRules and flatten them
// with ComposeUtxoValidationRules.
type UtxoValidationRuleGroup struct {
	rules           []UtxoValidationRuleFunc
	phase2ValidOnly bool
}

// AlwaysUtxoValidationRules groups UTXOW and other rules that must run for both
// phase-2-valid and phase-2-invalid transactions.
func AlwaysUtxoValidationRules(
	rules ...UtxoValidationRuleFunc,
) UtxoValidationRuleGroup {
	return UtxoValidationRuleGroup{rules: rules}
}

// Phase2ValidUtxoValidationRules groups certificate, governance, and other
// ledger rules whose transitions run only for phase-2-valid transactions.
func Phase2ValidUtxoValidationRules(
	rules ...UtxoValidationRuleFunc,
) UtxoValidationRuleGroup {
	return UtxoValidationRuleGroup{
		rules:           rules,
		phase2ValidOnly: true,
	}
}

// ComposeUtxoValidationRules flattens rule groups without changing their
// positions. Rules in a Phase2ValidUtxoValidationRules group become no-ops for
// phase-2-invalid transactions; always-run rules retain their original
// function values.
func ComposeUtxoValidationRules(
	groups ...UtxoValidationRuleGroup,
) []UtxoValidationRuleFunc {
	ruleCount := 0
	for _, group := range groups {
		ruleCount += len(group.rules)
	}
	ret := make([]UtxoValidationRuleFunc, 0, ruleCount)
	for _, group := range groups {
		if !group.phase2ValidOnly {
			ret = append(ret, group.rules...)
			continue
		}
		for _, rule := range group.rules {
			ret = append(ret, func(
				tx Transaction,
				slot uint64,
				ledgerState LedgerState,
				protocolParams ProtocolParameters,
			) error {
				if !tx.IsValid() {
					return nil
				}
				return rule(tx, slot, ledgerState, protocolParams)
			})
		}
	}
	return ret
}

// VerifyTransaction runs the provided validation rules in order and wraps
// the first error encountered into a ValidationError.
func VerifyTransaction(
	tx Transaction,
	slot uint64,
	ledgerState LedgerState,
	protocolParams ProtocolParameters,
	validationRules []UtxoValidationRuleFunc,
) error {
	if ledgerState != nil &&
		(reflect.ValueOf(ledgerState).Kind() != reflect.Pointer ||
			!reflect.ValueOf(ledgerState).IsNil()) {
		ledgerState = &cachedLedgerState{
			LedgerState: ledgerState,
			lookups:     make(map[string]cachedUtxoLookup),
		}
	}
	for i, rule := range validationRules {
		if err := rule(tx, slot, ledgerState, protocolParams); err != nil {
			details := map[string]any{"rule_index": i, "slot": slot}
			if tx != nil {
				details["tx_hash"] = tx.Hash().String()
			}
			return NewValidationError(
				ValidationErrorTypeTransaction,
				"transaction validation failed",
				details,
				err,
			)
		}
	}
	return nil
}

// txTypeAlonzo is the first era whose historical on-wire CBOR includes the
// 1-byte IsValid boolean field. The fee-relevant size excludes this byte only
// when the transaction actually has the 4-component Alonzo-style envelope.
const txTypeAlonzo = 4

// TxSize returns the size of a transaction as the Cardano ledger spec defines
// it — the measure used both for fees and for maxTxSize.
//
// For Alonzo through Conway the on-wire CBOR is a 4-element array
// [body, witnesses, isValid, metadata], while the Haskell
// toCBORForSizeComputation encodes only 3 elements. The IsValid flag is not
// part of the transaction the chain sized: a block stores bodies, witness
// sets, IsValid flags and auxiliary data in four parallel arrays, and
// reconstructing a standalone transaction re-attaches that byte. So the size
// is the full on-wire length minus 1. Dijkstra block transactions use a
// 3-element envelope, so no adjustment applies unless an explicitly
// 4-component transaction is being sized.
//
// Every rule expressed against a transaction's size must use this, not
// len(tx.Cbor()). The two differ by exactly one byte, which only matters for a
// transaction sitting exactly on a limit — rare enough to survive a long way
// into a replay before a single transaction built to fill maxTxSize exposes
// it.
//
// When the transaction has no stored CBOR (e.g. programmatically constructed),
// the function falls back to encoding the transaction to compute its size.
func TxSize(tx Transaction) (int, error) {
	cborData := tx.Cbor()
	if len(cborData) == 0 {
		// Fallback: encode the transaction to compute its size.
		// This handles programmatically constructed transactions
		// whose Cbor() returns nil because no stored CBOR exists.
		var err error
		cborData, err = cbor.Encode(tx)
		if err != nil {
			return 0, fmt.Errorf("failed to encode transaction for size: %w", err)
		}
	}
	fullSize := len(cborData)
	if tx.Type() >= txTypeAlonzo {
		dec, err := cbor.NewStreamDecoder(cborData)
		if err == nil {
			arrayLen, _, _, decodeErr := dec.DecodeArrayHeader()
			if decodeErr == nil && arrayLen == 4 {
				return fullSize - 1, nil
			}
		}
	}
	return fullSize, nil
}

// TxSizeForFee returns the fee-relevant size of a transaction. It is the same
// measure as TxSize; the name is kept for the fee rules that already call it.
func TxSizeForFee(tx Transaction) (int, error) {
	return TxSize(tx)
}

// CalculateMinFee computes the minimum fee for a transaction given its
// fee-relevant size (as returned by TxSizeForFee) and the protocol parameters
// MinFeeA and MinFeeB. It returns an error if the computation would
// overflow a uint64.
func CalculateMinFee(
	bodySize int,
	minFeeA uint,
	minFeeB uint,
) (uint64, error) {
	if bodySize < 0 {
		return 0, fmt.Errorf(
			"min fee: negative body size %d",
			bodySize,
		)
	}
	// Two-step conversion (int → uint → uint64) so that gosec G115 does
	// not flag the signed-to-unsigned cast: the negative guard above makes
	// int → uint safe, and uint → uint64 is a widening conversion.
	uBodySize := uint64(uint(bodySize))
	hi, lo := bits.Mul64(uint64(minFeeA), uBodySize)
	if hi != 0 {
		return 0, fmt.Errorf(
			"min fee overflow: %d * %d exceeds uint64",
			minFeeA,
			bodySize,
		)
	}
	sum, carry := bits.Add64(lo, uint64(minFeeB), 0)
	if carry != 0 {
		return 0, fmt.Errorf(
			"min fee overflow: %d + %d exceeds uint64",
			lo,
			minFeeB,
		)
	}
	return sum, nil
}

// Common witness-related error types for lightweight UTXOW checks.
type MissingVKeyWitnessesError struct{}

func (MissingVKeyWitnessesError) Error() string { return "missing required vkey witnesses" }

type MissingRequiredVKeyWitnessForSignerError struct{ Signer Blake2b224 }

func (e MissingRequiredVKeyWitnessForSignerError) Error() string {
	return fmt.Sprintf(
		"missing required vkey witness for required signer %x",
		e.Signer,
	)
}

// MalformedAuthorizationError reports a certificate or voter whose
// authorization requirement cannot be determined safely.
type MalformedAuthorizationError struct {
	Subject string
}

func (e MalformedAuthorizationError) Error() string {
	return "malformed authorization subject: " + e.Subject
}

// forEachCertificateCredential visits every credential whose authorization is
// carried by a transaction certificate. Only implicit-deposit registration does
// not require authorization; explicit-deposit registration requires the stake
// credential witness. Keeping this traversal shared prevents key and script
// credential requirements from diverging. Malformed certificates fail closed
// rather than silently omitting an authorization requirement.
//
// Every entry below is taken from getVKeyWitnessTxCert and
// getScriptWitnessTxCert in
// eras/shelley/impl/src/Cardano/Ledger/Shelley/TxCert.hs and
// eras/conway/impl/src/Cardano/Ledger/Conway/TxCert.hs. Conway shadows the
// Shelley definitions for every form it carries, so the Conway file is
// authoritative for tags 0-4 and 7-18 and the Shelley file for tags 5 and 6,
// which Conway expunges.
//
// Certificate authorization completeness (CBOR tags):
//
//   - 0: registration without an explicit deposit has no credential witness.
//     Conway decodes it to ConwayRegCert _ SNothing, which both accessors
//     match with Nothing, so its credential creates no script purpose either.
//   - 1, 2, 7-18: the certificate credential named below authorizes it. Tag 7
//     decodes to ConwayRegCert cred (SJust _), which the same two accessors
//     match with credKeyHashWitness cred and credScriptHash cred. The deposit
//     field, not the constructor, is what separates tag 7 from tag 0.
//   - 3: the pool operator is collected separately, and every pool owner is
//     collected by the independent owners term.
//   - 4: the retiring pool key is collected separately.
//   - 5: the genesis root key authorizes delegation; the new delegate and VRF
//     key are targets, not authors.
//   - 6: MIR has no field-level author; Shelley's accessor returns Nothing for
//     it. Its stateful genesis-delegate quorum is implemented by
//     ValidateMIRGenesisQuorum, which is not yet registered in any era rule
//     list; Conway expunges MIR.
//
// This switch deliberately names all 19 certificate forms so typed nils and a
// future unhandled implementation cannot silently bypass authorization.
func forEachCertificateCredential(
	cert Certificate,
	visit func(credential Credential, requiresWitness bool),
) error {
	visitCredential := func(credential Credential, requiresWitness bool) error {
		switch credential.CredType {
		case CredentialTypeAddrKeyHash, CredentialTypeScriptHash:
			visit(credential, requiresWitness)
			return nil
		default:
			return MalformedAuthorizationError{
				Subject: fmt.Sprintf(
					"certificate credential type %d",
					credential.CredType,
				),
			}
		}
	}
	typedNil := func(name string) error {
		return MalformedAuthorizationError{Subject: "nil " + name}
	}
	if cert == nil {
		return typedNil("certificate")
	}
	switch c := cert.(type) {
	case *StakeRegistrationCertificate:
		if c == nil {
			return typedNil("stake registration certificate")
		}
		return visitCredential(c.StakeCredential, false)
	case *StakeDeregistrationCertificate:
		if c == nil {
			return typedNil("stake deregistration certificate")
		}
		return visitCredential(c.StakeCredential, true)
	case *StakeDelegationCertificate:
		if c == nil {
			return typedNil("stake delegation certificate")
		}
		if c.StakeCredential == nil {
			return typedNil("stake delegation credential")
		}
		return visitCredential(*c.StakeCredential, true)
	case *RegistrationCertificate:
		if c == nil {
			return typedNil("registration certificate")
		}
		// Tag 7 carries an explicit deposit, so Conway decodes it to
		// ConwayRegCert cred (SJust _). getVKeyWitnessConwayTxCert and
		// getScriptWitnessConwayTxCert both match that pattern ahead of the
		// SNothing case and return the credential's witness, so unlike tag 0
		// this form is authenticated.
		return visitCredential(c.StakeCredential, true)
	case *DeregistrationCertificate:
		if c == nil {
			return typedNil("deregistration certificate")
		}
		return visitCredential(c.StakeCredential, true)
	case *VoteDelegationCertificate:
		if c == nil {
			return typedNil("vote delegation certificate")
		}
		return visitCredential(c.StakeCredential, true)
	case *StakeVoteDelegationCertificate:
		if c == nil {
			return typedNil("stake vote delegation certificate")
		}
		return visitCredential(c.StakeCredential, true)
	case *StakeRegistrationDelegationCertificate:
		if c == nil {
			return typedNil("stake registration delegation certificate")
		}
		return visitCredential(c.StakeCredential, true)
	case *VoteRegistrationDelegationCertificate:
		if c == nil {
			return typedNil("vote registration delegation certificate")
		}
		return visitCredential(c.StakeCredential, true)
	case *StakeVoteRegistrationDelegationCertificate:
		if c == nil {
			return typedNil("stake vote registration delegation certificate")
		}
		return visitCredential(c.StakeCredential, true)
	case *AuthCommitteeHotCertificate:
		if c == nil {
			return typedNil("committee hot authorization certificate")
		}
		return visitCredential(c.ColdCredential, true)
	case *ResignCommitteeColdCertificate:
		if c == nil {
			return typedNil("committee cold resignation certificate")
		}
		return visitCredential(c.ColdCredential, true)
	case *RegistrationDrepCertificate:
		if c == nil {
			return typedNil("DRep registration certificate")
		}
		return visitCredential(c.DrepCredential, true)
	case *DeregistrationDrepCertificate:
		if c == nil {
			return typedNil("DRep deregistration certificate")
		}
		return visitCredential(c.DrepCredential, true)
	case *UpdateDrepCertificate:
		if c == nil {
			return typedNil("DRep update certificate")
		}
		return visitCredential(c.DrepCredential, true)
	case *PoolRegistrationCertificate:
		if c == nil {
			return typedNil("pool registration certificate")
		}
		return nil
	case *PoolRetirementCertificate:
		if c == nil {
			return typedNil("pool retirement certificate")
		}
		return nil
	case *GenesisKeyDelegationCertificate:
		if c == nil {
			return typedNil("genesis key delegation certificate")
		}
		return nil
	case *MoveInstantaneousRewardsCertificate:
		if c == nil {
			return typedNil("instantaneous rewards certificate")
		}
		return nil
	default:
		return MalformedAuthorizationError{
			Subject: fmt.Sprintf("unsupported certificate type %T", cert),
		}
	}
}

type MissingRedeemersForScriptDataHashError struct{}

func (MissingRedeemersForScriptDataHashError) Error() string {
	return "missing redeemers for script data hash"
}

type MissingPlutusScriptWitnessesError struct{}

func (MissingPlutusScriptWitnessesError) Error() string {
	return "missing Plutus script witnesses for redeemers"
}

type ExtraneousPlutusScriptWitnessesError struct{}

func (ExtraneousPlutusScriptWitnessesError) Error() string {
	return "extraneous Plutus script witnesses"
}

type MissingScriptWitnessesError struct {
	ScriptHash ScriptHash
}

func (e MissingScriptWitnessesError) Error() string {
	return fmt.Sprintf(
		"missing script witness for script hash %x",
		e.ScriptHash[:],
	)
}

type ExtraneousScriptWitnessesError struct {
	ScriptHash ScriptHash
}

func (e ExtraneousScriptWitnessesError) Error() string {
	return fmt.Sprintf(
		"extraneous script witness for script hash %x",
		e.ScriptHash[:],
	)
}

// ValidateRequiredVKeyWitnesses checks that all required key credentials have a
// vkey witness. This includes explicitly required signers and key-based reward
// withdrawal credentials.
func ValidateRequiredVKeyWitnesses(tx Transaction) error {
	if err := ValidateWithdrawalAddresses(tx.Withdrawals()); err != nil {
		return err
	}
	required := make(
		map[Blake2b224]struct{},
		len(tx.RequiredSigners())+len(tx.Withdrawals()),
	)
	for _, signer := range tx.RequiredSigners() {
		required[signer] = struct{}{}
	}
	for addr := range tx.Withdrawals() {
		credential, err := addr.RewardAccountCredential()
		if err != nil {
			return err
		}
		if credential.CredType == CredentialTypeAddrKeyHash {
			required[credential.Credential] = struct{}{}
		}
	}
	for _, cert := range tx.Certificates() {
		if err := forEachCertificateCredential(cert, func(
			credential Credential,
			requiresWitness bool,
		) {
			if requiresWitness && credential.CredType == CredentialTypeAddrKeyHash {
				required[credential.Credential] = struct{}{}
			}
		}); err != nil {
			return err
		}
		switch c := cert.(type) {
		case *PoolRegistrationCertificate:
			if c == nil {
				continue
			}
			required[Blake2b224(c.Operator)] = struct{}{}
			for _, owner := range c.PoolOwners {
				required[Blake2b224(owner)] = struct{}{}
			}
		case *PoolRetirementCertificate:
			if c != nil {
				required[Blake2b224(c.PoolKeyHash)] = struct{}{}
			}
		case *GenesisKeyDelegationCertificate:
			if c != nil {
				if len(c.GenesisHash) != Blake2b224Size {
					return MalformedAuthorizationError{Subject: fmt.Sprintf(
						"genesis key hash length %d",
						len(c.GenesisHash),
					)}
				}
				required[NewBlake2b224(c.GenesisHash)] = struct{}{}
			}
		}
	}
	for voter := range tx.VotingProcedures() {
		credential, err := voterCredential(voter)
		if err != nil {
			return err
		}
		if credential.CredType == CredentialTypeAddrKeyHash {
			required[credential.Credential] = struct{}{}
		}
	}
	if len(required) == 0 {
		return nil
	}
	w := tx.Witnesses()
	if w == nil || len(w.Vkey()) == 0 {
		return MissingVKeyWitnessesError{}
	}
	vkeyHashes := make(map[Blake2b224]struct{}, len(w.Vkey()))
	for _, vw := range w.Vkey() {
		vkeyHashes[Blake2b224Hash(vw.Vkey)] = struct{}{}
	}
	for req := range required {
		if _, ok := vkeyHashes[req]; !ok {
			return MissingRequiredVKeyWitnessForSignerError{Signer: req}
		}
	}
	return nil
}

// ValidateMIRGenesisQuorum enforces the genesis-delegate quorum that authorizes
// a move instantaneous rewards certificate. MIR names no author in its own
// fields, so Shelley through Babbage require signatures from a quorum of the
// currently delegated genesis keys. A ledger state that cannot answer the
// query fails closed rather than admitting an unauthorized certificate.
func ValidateMIRGenesisQuorum(tx Transaction, ls LedgerState) error {
	hasMIR := false
	for _, cert := range tx.Certificates() {
		if _, ok := cert.(*MoveInstantaneousRewardsCertificate); ok {
			hasMIR = true
			break
		}
	}
	if !hasMIR {
		return nil
	}
	genesisState, ok := UnwrapLedgerState(ls).(GenesisDelegationState)
	if !ok {
		return GenesisDelegationStateUnavailableError{}
	}
	delegates, err := genesisState.GenesisDelegateKeyHashes()
	if err != nil {
		return err
	}
	quorum, err := genesisState.GenesisUpdateQuorum()
	if err != nil {
		return err
	}
	delegateSet := make(map[Blake2b224]struct{}, len(delegates))
	for _, delegate := range delegates {
		delegateSet[delegate] = struct{}{}
	}
	// Only a signature from a currently delegated genesis key counts, and each
	// delegate counts once no matter how often it appears in the witness set.
	signed := make(map[Blake2b224]struct{}, len(delegateSet))
	if w := tx.Witnesses(); w != nil {
		for _, vw := range w.Vkey() {
			hash := Blake2b224Hash(vw.Vkey)
			if _, ok := delegateSet[hash]; ok {
				signed[hash] = struct{}{}
			}
		}
	}
	if uint(len(signed)) < quorum {
		return MIRInsufficientGenesisSigsError{
			Provided: uint(len(signed)),
			Required: quorum,
		}
	}
	return nil
}

// ValidateUnsupportedPlutusExecution fails closed when a transaction requires
// phase-2 Plutus execution in an era that does not implement it.
func ValidateUnsupportedPlutusExecution(tx Transaction, era string) error {
	if !tx.IsValid() {
		return nil
	}
	wits := tx.Witnesses()
	if wits == nil || wits.Redeemers() == nil {
		return nil
	}
	for range wits.Redeemers().Iter() {
		return PlutusScriptValidationUnsupportedError{Era: era}
	}
	return nil
}

type scriptRequirement struct {
	hash     ScriptHash
	redeemer RedeemerKey
}

type transactionScriptRequirements struct {
	required    map[ScriptHash]struct{}
	purposes    []scriptRequirement
	explicit    map[ScriptHash]Script
	available   map[ScriptHash]Script
	nativeOrder []ScriptHash
}

func normalizeAvailableScript(script Script) (Script, error) {
	switch s := script.(type) {
	case NativeScript:
		return s, nil
	case *NativeScript:
		if s == nil {
			return nil, MalformedAuthorizationError{Subject: "nil native script"}
		}
		return *s, nil
	case PlutusV1Script:
		return s, nil
	case *PlutusV1Script:
		if s == nil {
			return nil, MalformedAuthorizationError{Subject: "nil Plutus V1 script"}
		}
		return *s, nil
	case PlutusV2Script:
		return s, nil
	case *PlutusV2Script:
		if s == nil {
			return nil, MalformedAuthorizationError{Subject: "nil Plutus V2 script"}
		}
		return *s, nil
	case PlutusV3Script:
		return s, nil
	case *PlutusV3Script:
		if s == nil {
			return nil, MalformedAuthorizationError{Subject: "nil Plutus V3 script"}
		}
		return *s, nil
	case PlutusV4Script:
		return s, nil
	case *PlutusV4Script:
		if s == nil {
			return nil, MalformedAuthorizationError{Subject: "nil Plutus V4 script"}
		}
		return *s, nil
	default:
		return nil, MalformedAuthorizationError{
			Subject: fmt.Sprintf("unsupported script type %T", script),
		}
	}
}

func addAvailableScript(
	dst map[ScriptHash]Script,
	script Script,
) (ScriptHash, error) {
	normalized, err := normalizeAvailableScript(script)
	if err != nil {
		return ScriptHash{}, err
	}
	hash := normalized.Hash()
	dst[hash] = normalized
	return hash, nil
}

func voterCredential(voter *Voter) (Credential, error) {
	if voter == nil {
		return Credential{}, MalformedAuthorizationError{Subject: "nil voter"}
	}
	credential := Credential{Credential: NewBlake2b224(voter.Hash[:])}
	switch voter.Type {
	case VoterTypeConstitutionalCommitteeHotKeyHash,
		VoterTypeDRepKeyHash,
		VoterTypeStakingPoolKeyHash:
		credential.CredType = CredentialTypeAddrKeyHash
	case VoterTypeConstitutionalCommitteeHotScriptHash,
		VoterTypeDRepScriptHash:
		credential.CredType = CredentialTypeScriptHash
	default:
		return Credential{}, MalformedAuthorizationError{
			Subject: fmt.Sprintf("voter type %d", voter.Type),
		}
	}
	return credential, nil
}

func voterPurposeOrder(voter *Voter) int {
	switch voter.Type {
	case VoterTypeConstitutionalCommitteeHotScriptHash:
		return 0
	case VoterTypeConstitutionalCommitteeHotKeyHash:
		return 1
	case VoterTypeDRepScriptHash:
		return 2
	case VoterTypeDRepKeyHash:
		return 3
	case VoterTypeStakingPoolKeyHash:
		return 4
	default:
		return -1
	}
}

func collectTransactionScriptRequirements(
	tx Transaction,
	ls LedgerState,
) (transactionScriptRequirements, error) {
	ret := transactionScriptRequirements{
		required:  make(map[ScriptHash]struct{}),
		explicit:  make(map[ScriptHash]Script),
		available: make(map[ScriptHash]Script),
	}
	addRequirement := func(hash ScriptHash, tag RedeemerTag, index int) {
		ret.required[hash] = struct{}{}
		ret.purposes = append(ret.purposes, scriptRequirement{
			hash: hash,
			redeemer: RedeemerKey{
				Tag:   tag,
				Index: uint32(index), // #nosec G115 -- transaction collections are bounded
			},
		})
	}
	addWitnessSet := func(wits TransactionWitnessSet) error {
		if wits == nil {
			return nil
		}
		for _, native := range wits.NativeScripts() {
			hash, err := addAvailableScript(ret.available, native)
			if err != nil {
				return err
			}
			ret.explicit[hash] = ret.available[hash]
			ret.nativeOrder = append(ret.nativeOrder, hash)
		}
		for _, plutus := range wits.PlutusV1Scripts() {
			hash, err := addAvailableScript(ret.available, plutus)
			if err != nil {
				return err
			}
			ret.explicit[hash] = ret.available[hash]
		}
		for _, plutus := range wits.PlutusV2Scripts() {
			hash, err := addAvailableScript(ret.available, plutus)
			if err != nil {
				return err
			}
			ret.explicit[hash] = ret.available[hash]
		}
		for _, plutus := range wits.PlutusV3Scripts() {
			hash, err := addAvailableScript(ret.available, plutus)
			if err != nil {
				return err
			}
			ret.explicit[hash] = ret.available[hash]
		}
		for _, plutus := range PlutusV4ScriptsFromWitnessSet(wits) {
			hash, err := addAvailableScript(ret.available, plutus)
			if err != nil {
				return err
			}
			ret.explicit[hash] = ret.available[hash]
		}
		return nil
	}
	if err := addWitnessSet(tx.Witnesses()); err != nil {
		return ret, err
	}

	resolvedInputs := make(map[string]Utxo, len(tx.Inputs()))
	if ls != nil {
		for _, input := range tx.Inputs() {
			utxo, err := ls.UtxoById(input)
			if err != nil {
				// BadInputsUtxo reports unresolved consumed inputs before script
				// evaluation. Preserve that error precedence here.
				continue
			}
			resolvedInputs[input.String()] = utxo
			if utxo.Output != nil && utxo.Output.ScriptRef() != nil {
				if _, err := addAvailableScript(
					ret.available,
					utxo.Output.ScriptRef(),
				); err != nil {
					return ret, err
				}
			}
		}
		for _, input := range tx.ReferenceInputs() {
			utxo, err := ls.UtxoById(input)
			if err != nil {
				return ret, ReferenceInputResolutionError{Input: input, Err: err}
			}
			if utxo.Output != nil && utxo.Output.ScriptRef() != nil {
				if _, err := addAvailableScript(
					ret.available,
					utxo.Output.ScriptRef(),
				); err != nil {
					return ret, err
				}
			}
		}
	}

	inputs := append([]TransactionInput(nil), tx.Inputs()...)
	sort.Slice(inputs, func(i, j int) bool {
		if cmp := bytes.Compare(inputs[i].Id().Bytes(), inputs[j].Id().Bytes()); cmp != 0 {
			return cmp < 0
		}
		return inputs[i].Index() < inputs[j].Index()
	})
	for index, input := range inputs {
		utxo, ok := resolvedInputs[input.String()]
		if !ok || utxo.Output == nil {
			continue
		}
		addr := utxo.Output.Address()
		if addr.Type()&AddressTypeScriptBit != 0 {
			addRequirement(
				ScriptHash(addr.PaymentKeyHash()),
				RedeemerTagSpend,
				index,
			)
		}
	}

	if mint := tx.AssetMint(); mint != nil {
		policies := mint.Policies()
		sort.Slice(policies, func(i, j int) bool {
			return bytes.Compare(policies[i].Bytes(), policies[j].Bytes()) < 0
		})
		for index, policy := range policies {
			addRequirement(ScriptHash(policy), RedeemerTagMint, index)
		}
	}

	for index, cert := range tx.Certificates() {
		if err := forEachCertificateCredential(cert, func(
			credential Credential,
			requiresWitness bool,
		) {
			if !requiresWitness ||
				credential.CredType != CredentialTypeScriptHash {
				return
			}
			addRequirement(
				ScriptHash(credential.Credential),
				RedeemerTagCert,
				index,
			)
		}); err != nil {
			return ret, err
		}
	}

	// Reward redeemer indices follow cardano-ledger's Withdrawals key order,
	// which is the derived Ord on AccountAddress: (Network, Credential). See
	// AccountAddress and the RewardAccount pattern in
	// libs/cardano-ledger-core/src/Cardano/Ledger/Address.hs, Network in
	// libs/cardano-ledger-core/src/Cardano/Ledger/BaseTypes.hs (Testnet before
	// Mainnet), and Credential in
	// libs/cardano-ledger-core/src/Cardano/Ledger/Credential.hs, whose derived
	// Ord puts ScriptHashObj before KeyHashObj. A script credential therefore
	// sorts ahead of a key credential with the same hash. Address bytes invert
	// that, because the reward header is 0xE_ for a key hash and 0xF_ for a
	// script hash, so sorting on them would give a script withdrawal a
	// different index than the node assigns. voterPurposeOrder below encodes
	// the same script-before-key rule.
	type withdrawalKey struct {
		credential Credential
		network    uint
	}
	withdrawals := make([]withdrawalKey, 0, len(tx.Withdrawals()))
	for addr := range tx.Withdrawals() {
		credential, err := addr.RewardAccountCredential()
		if err != nil {
			return ret, err
		}
		withdrawals = append(withdrawals, withdrawalKey{
			credential: credential,
			network:    addr.NetworkId(),
		})
	}
	credentialOrder := func(credential Credential) int {
		if credential.CredType == CredentialTypeScriptHash {
			return 0
		}
		return 1
	}
	sort.Slice(withdrawals, func(i, j int) bool {
		if withdrawals[i].network != withdrawals[j].network {
			return withdrawals[i].network < withdrawals[j].network
		}
		iOrder := credentialOrder(withdrawals[i].credential)
		jOrder := credentialOrder(withdrawals[j].credential)
		if iOrder != jOrder {
			return iOrder < jOrder
		}
		return bytes.Compare(
			withdrawals[i].credential.Credential[:],
			withdrawals[j].credential.Credential[:],
		) < 0
	})
	for index, entry := range withdrawals {
		credential := entry.credential
		if credential.CredType == CredentialTypeScriptHash {
			addRequirement(
				ScriptHash(credential.Credential),
				RedeemerTagReward,
				index,
			)
		}
	}

	voters := make([]*Voter, 0, len(tx.VotingProcedures()))
	for voter := range tx.VotingProcedures() {
		if _, err := voterCredential(voter); err != nil {
			return ret, err
		}
		voters = append(voters, voter)
	}
	sort.Slice(voters, func(i, j int) bool {
		iOrder := voterPurposeOrder(voters[i])
		jOrder := voterPurposeOrder(voters[j])
		if iOrder != jOrder {
			return iOrder < jOrder
		}
		return bytes.Compare(voters[i].Hash[:], voters[j].Hash[:]) < 0
	})
	for index, voter := range voters {
		credential, err := voterCredential(voter)
		if err != nil {
			return ret, err
		}
		if credential.CredType == CredentialTypeScriptHash {
			addRequirement(
				ScriptHash(credential.Credential),
				RedeemerTagVoting,
				index,
			)
		}
	}

	for index, proposal := range tx.ProposalProcedures() {
		if proposal == nil {
			return ret, MalformedAuthorizationError{Subject: "nil proposal procedure"}
		}
		govAction := proposal.GovAction()
		if govAction == nil {
			return ret, MalformedAuthorizationError{Subject: "nil governance action"}
		}
		if actionWithPolicy, ok := govAction.(GovActionWithPolicy); ok {
			policyHash := actionWithPolicy.GetPolicyHash()
			if len(policyHash) == Blake2b224Size {
				var hash ScriptHash
				copy(hash[:], policyHash)
				addRequirement(hash, RedeemerTagProposing, index)
			} else if policyHash != nil {
				// Present but invalid length - fail fast to surface upstream bugs
				return ret, fmt.Errorf(
					"malformed governance policy hash: got %d bytes, want %d",
					len(policyHash),
					Blake2b224Size,
				)
			}
		}
	}
	return ret, nil
}

// NativeScriptsForValidation returns all explicit native scripts plus every
// native reference script required by a concrete transaction purpose. The
// latter must be evaluated even though it is not carried in the witness set.
func NativeScriptsForValidation(
	tx Transaction,
	ls LedgerState,
) ([]NativeScript, error) {
	requirements, err := collectTransactionScriptRequirements(tx, ls)
	if err != nil {
		return nil, err
	}
	needed := make(map[ScriptHash]NativeScript)
	for _, hash := range requirements.nativeOrder {
		if script, ok := requirements.explicit[hash].(NativeScript); ok {
			needed[hash] = script
		}
	}
	for hash := range requirements.required {
		if script, ok := requirements.available[hash].(NativeScript); ok {
			needed[hash] = script
		}
	}
	hashes := make([]ScriptHash, 0, len(needed))
	for hash := range needed {
		hashes = append(hashes, hash)
	}
	sort.Slice(hashes, func(i, j int) bool {
		return bytes.Compare(hashes[i][:], hashes[j][:]) < 0
	})
	ret := make([]NativeScript, 0, len(hashes))
	for _, hash := range hashes {
		ret = append(ret, needed[hash])
	}
	return ret, nil
}

// ValidateScriptWitnesses checks script availability and requires the exact
// redeemer pointer for every Plutus purpose. Native purposes reject a redeemer
// at that pointer and are evaluated separately, including reference scripts.
func ValidateScriptWitnesses(tx Transaction, ls LedgerState) error {
	if err := ValidateWithdrawalAddresses(tx.Withdrawals()); err != nil {
		return err
	}
	if ls == nil {
		// Without ledger state a reference script cannot be resolved, so every
		// requirement it would have satisfied would be reported missing. Main
		// returned early here and this keeps that contract rather than
		// tightening it as a side effect.
		return nil
	}
	if !tx.IsValid() {
		return nil
	}
	requirements, err := collectTransactionScriptRequirements(tx, ls)
	if err != nil {
		return err
	}
	for required := range requirements.required {
		if _, ok := requirements.available[required]; !ok {
			return MissingScriptWitnessesError{ScriptHash: required}
		}
	}
	for provided := range requirements.explicit {
		if _, ok := requirements.required[provided]; !ok {
			// A witness-set script with no script purpose is extraneous. See
			// validateMissingScripts in
			// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Utxow.hs and
			// babbageMissingScripts in
			// eras/babbage/impl/src/Cardano/Ledger/Babbage/Rules/Utxow.hs,
			// which both fail on sProvided minus the needed set. Tag-0
			// registration creates no purpose, so a script matching its
			// credential lands here.
			return ExtraneousScriptWitnessesError{ScriptHash: provided}
		}
	}

	redeemers := map[RedeemerKey]struct{}{}
	if wits := tx.Witnesses(); wits != nil && wits.Redeemers() != nil {
		for key := range wits.Redeemers().Iter() {
			redeemers[key] = struct{}{}
		}
	}
	for _, purpose := range requirements.purposes {
		available := requirements.available[purpose.hash]
		_, hasRedeemer := redeemers[purpose.redeemer]
		if _, isPlutus := PlutusScriptVersion(available); isPlutus {
			if !hasRedeemer {
				return MissingRedeemerForScriptError{
					ScriptHash:  purpose.hash,
					Tag:         purpose.redeemer.Tag,
					Index:       purpose.redeemer.Index,
					RedeemerKey: purpose.redeemer,
				}
			}
			continue
		}
		if _, isNative := available.(NativeScript); isNative && hasRedeemer {
			return ExtraneousRedeemerError{RedeemerKey: purpose.redeemer}
		}
	}
	return nil
}

// ValidateExtraneousRedeemers checks that every redeemer in the
// transaction's witness set has a tag/index that maps to a real script
// purpose: a spending redeemer must index an existing input, a minting
// redeemer an existing (distinct, sorted) mint policy, a certifying
// redeemer an existing certificate, a reward redeemer an existing
// withdrawal, a voting redeemer an existing voter, and a proposing redeemer
// an existing proposal procedure. Any redeemer whose index is out of range
// for its tag's category, or whose tag is not one of the above (e.g.
// RedeemerTagGuarding, which this shared check always treats as
// extraneous), causes ExtraneousRedeemerError to be returned for that
// redeemer. Eras that define additional redeemer purposes (e.g. Dijkstra's
// guarding redeemers) must check for and accept those before delegating the
// remaining redeemers to this function, since it fails closed on anything
// it doesn't recognize.
func ValidateExtraneousRedeemers(tx Transaction) error {
	wits := tx.Witnesses()
	if wits == nil {
		return nil
	}
	redeemers := wits.Redeemers()
	if redeemers == nil {
		return nil
	}

	// Get counts for each purpose type
	inputCount := len(tx.Inputs())
	certCount := len(tx.Certificates())
	withdrawalCount := len(tx.Withdrawals())
	proposalCount := len(tx.ProposalProcedures())

	// Count distinct mint policies
	mintPolicyCount := 0
	if mint := tx.AssetMint(); mint != nil {
		mintPolicyCount = len(mint.Policies())
	}

	// Count voters (each voter is a separate purpose index)
	voterCount := 0
	if votingProcs := tx.VotingProcedures(); votingProcs != nil {
		voterCount = len(votingProcs)
	}

	// Check each redeemer
	for redeemerKey := range redeemers.Iter() {
		var maxIndex uint64
		switch redeemerKey.Tag {
		case RedeemerTagSpend:
			maxIndex = countToUint64(inputCount)
		case RedeemerTagMint:
			maxIndex = countToUint64(mintPolicyCount)
		case RedeemerTagCert:
			maxIndex = countToUint64(certCount)
		case RedeemerTagReward:
			maxIndex = countToUint64(withdrawalCount)
		case RedeemerTagVoting:
			maxIndex = countToUint64(voterCount)
		case RedeemerTagProposing:
			maxIndex = countToUint64(proposalCount)
		case RedeemerTagGuarding:
			return ExtraneousRedeemerError{RedeemerKey: redeemerKey}
		default:
			// Any unrecognized tag doesn't map to a purpose this shared
			// check understands.
			return ExtraneousRedeemerError{RedeemerKey: redeemerKey}
		}

		if uint64(redeemerKey.Index) >= maxIndex {
			return ExtraneousRedeemerError{RedeemerKey: redeemerKey}
		}
	}

	return nil
}

// countToUint64 converts a collection length for comparison with a wire-width
// index. Collection lengths are non-negative and cannot exceed uint64.
func countToUint64(count int) uint64 {
	if count <= 0 {
		return 0
	}
	return uint64(count) //nolint:gosec // count is derived from len
}

// ValidateRedeemerAndScriptWitnesses performs lightweight checks between redeemers and Plutus scripts.
func ValidateRedeemerAndScriptWitnesses(tx Transaction, ls LedgerState) error {
	wits := tx.Witnesses()
	redeemerCount := 0
	if wits != nil {
		if r := wits.Redeemers(); r != nil {
			for range r.Iter() {
				redeemerCount++
			}
		}
	}
	hasPlutus := false
	if wits != nil {
		if len(wits.PlutusV1Scripts()) > 0 || len(wits.PlutusV2Scripts()) > 0 ||
			len(wits.PlutusV3Scripts()) > 0 ||
			len(PlutusV4ScriptsFromWitnessSet(wits)) > 0 {
			hasPlutus = true
		}
	}

	// If there are inputs (reference or regular) and a LedgerState is provided,
	// resolve them to detect Plutus reference scripts. Per CIP-33, ScriptRef can
	// be provided via both reference inputs AND regular (spent) inputs.
	hasPlutusReference := false
	if ls != nil {
		// Check reference inputs
		for _, refInput := range tx.ReferenceInputs() {
			utxo, err := ls.UtxoById(refInput)
			if err != nil {
				return ReferenceInputResolutionError{Input: refInput, Err: err}
			}
			if utxo.Output == nil {
				continue
			}
			script := utxo.Output.ScriptRef()
			if script == nil {
				continue
			}
			if _, ok := PlutusScriptVersion(script); ok {
				hasPlutusReference = true
			}
			if hasPlutusReference {
				break
			}
		}
		// Check regular inputs if not found in reference inputs
		if !hasPlutusReference {
			for _, input := range tx.Inputs() {
				utxo, err := ls.UtxoById(input)
				if err != nil {
					// Skip errors - BadInputsUtxo will catch this
					continue
				}
				if utxo.Output == nil {
					continue
				}
				script := utxo.Output.ScriptRef()
				if script == nil {
					continue
				}
				if _, ok := PlutusScriptVersion(script); ok {
					hasPlutusReference = true
				}
				if hasPlutusReference {
					break
				}
			}
		}
	}

	// Check witness PlutusData (datums)
	hasWitnessPlutusData := false
	if wits != nil {
		if len(wits.PlutusData()) > 0 {
			hasWitnessPlutusData = true
		}
	}

	// ScriptDataHash covers redeemers, datums, and language views.
	// It's valid to have ScriptDataHash with no redeemers if there are witness datums.
	if tx.ScriptDataHash() != nil && redeemerCount == 0 &&
		!hasWitnessPlutusData {
		return MissingRedeemersForScriptDataHashError{}
	}
	if redeemerCount > 0 && !hasPlutus && !hasPlutusReference {
		return MissingPlutusScriptWitnessesError{}
	}
	if redeemerCount == 0 && hasPlutus {
		return ExtraneousPlutusScriptWitnessesError{}
	}
	return nil
}

// EncodeLangViews encodes language views per the Cardano ledger specification.
// For PlutusV1, the tag is double-serialized and the cost model uses indefinite-length list.
// For PlutusV2+, the tag is single-serialized and the cost model uses definite-length list.
// The map is sorted by "shortLex" order (length first, then lexicographic).
func EncodeLangViews(
	usedVersions map[uint]struct{},
	costModels map[uint][]int64,
) ([]byte, error) {
	type langView struct {
		tag    []byte
		params []byte
	}

	views := make([]langView, 0, len(usedVersions))

	for version := range usedVersions {
		switch version {
		case 0, 1, 2, 3:
		default:
			return nil, fmt.Errorf(
				"unsupported Plutus version for lang views: %d",
				version,
			)
		}

		costModel, ok := costModels[version]
		if !ok {
			return nil, fmt.Errorf(
				"missing cost model for Plutus version: %d",
				version,
			)
		}

		var tag []byte
		var params []byte
		var err error

		switch version {
		case 0: // PlutusV1
			// Tag is double-serialized: serialize(serialize(0)) => 0x4100.
			tag = []byte{0x41, 0x00}
			// Cost model uses indefinite-length list, wrapped in a bytestring
			// This is the "double bagging" for PlutusV1 compatibility
			indefList := make(cbor.IndefLengthList, len(costModel))
			for i, v := range costModel {
				indefList[i] = any(v)
			}
			indefBytes, indefErr := cbor.Encode(indefList)
			if indefErr != nil {
				return nil, indefErr
			}
			// Wrap the indefinite list bytes in a CBOR bytestring
			params, err = cbor.Encode(indefBytes)
			if err != nil {
				return nil, err
			}

		case 1, 2, 3: // PlutusV2, PlutusV3, PlutusV4
			// Tags are single-byte CBOR encodings for small unsigned ints.
			tag = []byte{byte(version)}
			// Cost model uses definite-length list (no bytestring wrapper)
			params, err = cbor.Encode(costModel)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unsupported Plutus version for lang views: %d", version)
		}

		views = append(views, langView{tag: tag, params: params})
	}

	// Sort by "shortLex" order (length first, then lexicographic)
	sort.Slice(views, func(i, j int) bool {
		return ShortLex(views[i].tag, views[j].tag) < 0
	})

	totalSize := 1
	for _, v := range views {
		totalSize += len(v.tag) + len(v.params)
	}

	// Encode as a map with map length prefix
	result := make([]byte, 0, totalSize)
	// Encode map length (definite-length map)
	if len(views) < 24 {
		result = append(result, 0xa0+byte(len(views))) //nolint:gosec // len < 24
	} else {
		result = append(result, 0xb8, byte(len(views))) //nolint:gosec // len < 256
	}

	// Append key-value pairs in sorted order
	for _, v := range views {
		result = append(result, v.tag...)
		result = append(result, v.params...)
	}

	return result, nil
}

// ShortLex compares byte slices by length first, then lexicographically
func ShortLex(a, b []byte) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
