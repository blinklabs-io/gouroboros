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

package cbor

import (
	"bytes"
	"errors"
	"fmt"
	"unsafe"
)

const (
	defaultDiagnosticBytes = 8 * maxDiagnosticInputBytes
	// Include the allocated node, child slice growth, Value aggregates and
	// decoder scratch. Payload allocations are charged separately.
	diagnosticNodeBytes = 8*int(unsafe.Sizeof(DiagnosticNode{})) + 512
)

type diagnosticBudget struct {
	limits DiagnosticParseLimits
	nodes  int
	items  int
	bytes  int
	work   int
}

func (d *StreamDecoder) startDiagnostic(limits DiagnosticParseLimits) error {
	if d.diagnostic != nil {
		return nil
	}
	fields := []*int{
		&limits.MaxNodes, &limits.MaxCollectionItems,
		&limits.MaxRetainedBytes, &limits.MaxWorkBytes,
	}
	defaults := []int{
		defaultDiagnosticBytes / diagnosticNodeBytes,
		defaultDiagnosticBytes / diagnosticNodeBytes,
		defaultDiagnosticBytes, defaultDiagnosticBytes,
	}
	for i, field := range fields {
		if *field < 0 {
			return errors.New("negative diagnostic parse limit")
		}
		if *field == 0 {
			*field = defaults[i]
		}
	}
	budget := &diagnosticBudget{limits: limits}
	if err := budget.retain(len(d.data)); err != nil {
		return err
	}
	if err := budget.process(len(d.data)); err != nil {
		return err
	}
	position := d.Position()
	d.data = bytes.Clone(d.data)
	d.consumed = position
	d.dec = d.decMode.NewDecoder(bytes.NewReader(d.data[position:]))
	d.diagnostic = budget
	return nil
}

func (b *diagnosticBudget) retain(n int) error {
	if n < 0 || n > b.limits.MaxRetainedBytes-b.bytes {
		return errors.New("diagnostic retained-byte budget exhausted")
	}
	b.bytes += n
	return nil
}

func (b *diagnosticBudget) process(n int) error {
	if n < 0 || n > b.limits.MaxWorkBytes-b.work {
		return errors.New("diagnostic work budget exhausted")
	}
	b.work += n
	return nil
}

func (b *diagnosticBudget) node() error {
	if b.nodes >= b.limits.MaxNodes {
		return errors.New("diagnostic node budget exhausted")
	}
	if err := b.retain(diagnosticNodeBytes); err != nil {
		return err
	}
	if err := b.process(1); err != nil {
		return err
	}
	b.nodes++
	return nil
}

func (b *diagnosticBudget) collection(n int) error {
	if n < 0 || n > b.limits.MaxCollectionItems-b.items {
		return errors.New("diagnostic collection budget exhausted")
	}
	return nil
}

func (b *diagnosticBudget) item() error {
	if err := b.collection(1); err != nil {
		return err
	}
	b.items++
	return nil
}

func (b *diagnosticBudget) payload(n int, copies int) error {
	if n < 0 || n > (b.limits.MaxRetainedBytes-b.bytes)/copies {
		return errors.New("diagnostic payload budget exhausted")
	}
	if err := b.retain(n * copies); err != nil {
		return err
	}
	return b.process(n * copies)
}

func (d *StreamDecoder) admitDiagnosticNode() error {
	if err := d.startDiagnostic(DiagnosticParseLimits{}); err != nil {
		return err
	}
	if err := d.diagnostic.node(); err != nil {
		return err
	}
	position := d.Position()
	if position >= len(d.data) {
		return nil
	}
	header := d.data[position]
	major := header & CborTypeMask
	if (major == CborTypeByteString || major == CborTypeTextString) &&
		header&31 != 31 {
		length, _, _, err := parseCollectionHeader(d.data, position)
		if err != nil {
			return err
		}
		// Typed string decoding holds its payload plus growing read scratch.
		if err := d.diagnostic.payload(length, 4); err != nil {
			return fmt.Errorf("diagnostic string allocation: %w", err)
		}
	}
	return nil
}
