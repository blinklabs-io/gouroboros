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

// Test-only helpers: production evaluation uses the prepared-program path
// (decodePreparedPlutusProgram and runPooledPreparedContext). These decode a
// program the way that path does and expose the plain RunScriptContext
// runner, so tests can inspect programs and run caller-built terms.

import (
	"context"
	"errors"

	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
)

// decodePlutusProgram returns the decoded and execution-validated program
// for innerScript, applying the checks decodePreparedPlutusProgram applies.
// It is not cached.
func decodePlutusProgram(
	innerScript []byte,
	ledgerLanguage lang.LanguageVersion,
	evalContext *cek.EvalContext,
) (*syn.Program[syn.DeBruijn], error) {
	if evalContext == nil {
		return nil, errors.New("evaluation context is required")
	}
	programContext := syn.ProgramContext{
		LedgerLanguage: ledgerLanguage,
		ProtocolMajor:  evalContext.ProtoMajor,
	}
	program, err := syn.DecodeDeBruijnWithContext(innerScript, programContext)
	if err != nil {
		return nil, err
	}
	if err := syn.ValidateTermVersionForExecution(
		program.Version,
		programContext,
	); err != nil {
		return nil, err
	}
	return program, nil
}

// runPooledMachineContext runs program on a Machine checked out for (version,
// evalContext) with the given budget and returns the unit-result flag and
// budget consumption, which are computed even when Run fails. Trusted data
// arguments must be finite, valid values decoded from or built from ledger
// state; without arguments, the whole program tree is validated.
//
// The Machine is released only after Run has returned and its ExBudget has
// been read, never from a defer: a Run that panics leaves the Machine out of
// its pool rather than handing a Machine interrupted mid-evaluation to the
// next caller, and releasing before the ExBudget read would let a concurrent
// checkout overwrite it.
func runPooledMachineContext(
	ctx context.Context,
	version lang.LanguageVersion,
	evalContext *cek.EvalContext,
	budget cek.ExBudget,
	program syn.Term[syn.DeBruijn],
	trustedDataArguments ...data.PlutusData,
) (cek.ExBudget, bool, error) {
	machine := checkoutMachineWithBudget(version, evalContext, budget)
	// RunScriptContext never discharges the final value, as the reference
	// ledger does not: validation must not depend on plutigo's discharge bound.
	isUnit, runErr := machine.RunScriptContext(
		ctx,
		program,
		trustedDataArguments...,
	)
	consumed := budget.Sub(&machine.ExBudget)
	releaseMachine(version, evalContext, machine)
	return consumed, isUnit, runErr
}
