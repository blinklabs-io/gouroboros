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

// Class guard for optional ledger-state capabilities.
//
// common.VerifyTransaction hands each rule a transaction-scoped UTxO cache
// wrapper rather than the state the caller supplied. The wrapper embeds
// common.LedgerState, so it satisfies that interface and every interface
// LedgerState includes, but it does not satisfy the optional capability
// interfaces the concrete state may also implement. A rule that asserts one of
// those directly silently loses the capability and degrades: for
// common.EpochState that changes a pool deposit's accept/reject verdict, and
// for common.GovPurposeRootsState it weakens the Conway ancestry rule.
//
// The failure is invisible - it produces no compile error, no conflict on
// merge, and no failure in the rule's own unit tests, which call the rule
// directly rather than through VerifyTransaction. This test reads the rule
// sources instead, so a new rule or a rule merged from another branch is
// covered without anyone remembering to extend a table.

package ledger_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const commonStateFile = "common/state.go"

// ledgerStateInterfaces returns every interface declared in
// ledger/common/state.go, mapped to the interfaces it embeds.
func ledgerStateInterfaces(t *testing.T) map[string][]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, commonStateFile, nil, parser.SkipObjectResolution)
	require.NoError(t, err)
	ret := map[string][]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		iface, ok := spec.Type.(*ast.InterfaceType)
		if !ok {
			return true
		}
		embeds := []string{}
		for _, field := range iface.Methods.List {
			if len(field.Names) > 0 {
				continue
			}
			if ident, ok := field.Type.(*ast.Ident); ok {
				embeds = append(embeds, ident.Name)
			}
		}
		ret[spec.Name.Name] = embeds
		return true
	})
	return ret
}

// optionalCapabilities returns the interfaces in ledger/common/state.go that a
// value satisfying common.LedgerState is not guaranteed to implement. These are
// exactly the assertions the cache wrapper cannot preserve.
func optionalCapabilities(t *testing.T) []string {
	t.Helper()
	interfaces := ledgerStateInterfaces(t)
	require.Contains(t, interfaces, "LedgerState")
	included := map[string]bool{"LedgerState": true}
	queue := []string{"LedgerState"}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		for _, embed := range interfaces[name] {
			if included[embed] {
				continue
			}
			included[embed] = true
			queue = append(queue, embed)
		}
	}
	ret := []string{}
	for name := range interfaces {
		if !included[name] {
			ret = append(ret, name)
		}
	}
	sort.Strings(ret)
	return ret
}

// assertedInterfaceName returns the interface name a type assertion targets,
// for both the in-package (EpochState) and cross-package (common.EpochState)
// spellings.
func assertedInterfaceName(expr ast.Expr) string {
	switch typ := expr.(type) {
	case *ast.Ident:
		return typ.Name
	case *ast.SelectorExpr:
		pkg, ok := typ.X.(*ast.Ident)
		if !ok || pkg.Name != "common" {
			return ""
		}
		return typ.Sel.Name
	}
	return ""
}

// isUnwrapCall reports whether expr is a call to common.UnwrapLedgerState.
func isUnwrapCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name == "UnwrapLedgerState"
	case *ast.SelectorExpr:
		pkg, ok := fun.X.(*ast.Ident)
		return ok && pkg.Name == "common" && fun.Sel.Name == "UnwrapLedgerState"
	}
	return false
}

// TestOptionalCapabilityAssertionsUnwrapCachedState fails when a rule asserts
// an optional ledger-state capability without calling
// common.UnwrapLedgerState first. The capability set is derived from
// ledger/common/state.go rather than listed here, so adding a capability
// extends the guard automatically.
func TestOptionalCapabilityAssertionsUnwrapCachedState(t *testing.T) {
	t.Parallel()
	capabilities := map[string]bool{}
	for _, name := range optionalCapabilities(t) {
		capabilities[name] = true
	}
	require.NotEmpty(t, capabilities, "no optional capabilities found")
	// The capabilities that change an accept/reject verdict when lost.
	for _, expected := range []string{
		"EpochState",
		"GenesisDelegationState",
		"ClassicProtocolParameterUpdateWindowState",
		"GovPurposeRootsState",
		"StakeCredentialDepositState",
	} {
		require.True(
			t,
			capabilities[expected],
			"%s should be an optional capability",
			expected,
		)
	}

	fset := token.NewFileSet()
	var offenders []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" || d.Name() == ".worktrees" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			assertion, ok := n.(*ast.TypeAssertExpr)
			if !ok || assertion.Type == nil {
				return true
			}
			if !capabilities[assertedInterfaceName(assertion.Type)] {
				return true
			}
			if isUnwrapCall(assertion.X) {
				return true
			}
			offenders = append(
				offenders,
				fset.Position(assertion.Pos()).String()+
					": assertion to "+
					assertedInterfaceName(assertion.Type)+
					" is not applied to common.UnwrapLedgerState(...)",
			)
			return true
		})
		return nil
	})
	require.NoError(t, err)
	require.Empty(
		t,
		offenders,
		"optional ledger-state capabilities must be asserted on"+
			" common.UnwrapLedgerState(...); VerifyTransaction supplies a"+
			" cache wrapper that does not implement them",
	)
}
