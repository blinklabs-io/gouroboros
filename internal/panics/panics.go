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

// Package panics converts a panic raised inside a goroutine this library owns
// into an ordinary error, so that a fault reaches the caller's error handling
// instead of terminating the process.
package panics

import (
	"fmt"
	"runtime/debug"
)

// Recovered is the error a contained panic becomes. Sentinel makes the error
// matchable with errors.Is by the package that installed the guard, Value is
// whatever was passed to panic, and Stack is the stack of the panicking
// goroutine as captured at the recovery point.
type Recovered struct {
	Sentinel error
	Where    string
	Value    any
	Stack    []byte
}

// Error renders the panic value and the captured stack. The stack is part of
// the message rather than a side channel because a contained panic is the only
// record of a fault that would otherwise have produced a process-wide crash
// dump; losing it would turn a diagnosable bug into an unexplained
// disconnection.
func (e *Recovered) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf(
		"%s: recovered panic: %v\n%s",
		e.Where,
		e.Value,
		e.Stack,
	)
}

// Is reports whether this error matches the sentinel supplied at the guard
// site, so callers can test for a contained panic without naming this type.
func (e *Recovered) Is(target error) bool {
	return e != nil && e.Sentinel != nil && e.Sentinel == target
}

// Unwrap exposes the panic value when it is itself an error, and nil
// otherwise. A nil result ends the errors.Is chain without matching, which is
// correct: a panic with a non-error value wraps nothing.
func (e *Recovered) Unwrap() error {
	if e == nil {
		return nil
	}
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}

// New builds the error for a recovered value, or returns nil when there was no
// panic. value must come from a recover() call made directly by a deferred
// function. Since Go 1.21 a panic(nil) surfaces as *runtime.PanicNilError, so
// a nil value here always means "did not panic".
func New(sentinel error, where string, value any) error {
	if value == nil {
		return nil
	}
	return &Recovered{
		Sentinel: sentinel,
		Where:    where,
		Value:    value,
		Stack:    debug.Stack(),
	}
}

// Guard contains a panic raised in the function that deferred it, storing the
// resulting error through errOut. Deferring Guard directly is required for
// recover to see the panic:
//
//	func f() (err error) {
//		defer panics.Guard(ErrSomething, "where", &err)
//		...
//	}
//
// errOut is left untouched when the function returns normally, so a guarded
// function's own error return is never overwritten.
func Guard(sentinel error, where string, errOut *error) {
	err := New(sentinel, where, recover())
	if err != nil && errOut != nil {
		*errOut = err
	}
}
