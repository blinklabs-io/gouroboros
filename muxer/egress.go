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

package muxer

import (
	"errors"
	"sync"
	"time"
)

// EgressClass is the outbound traffic class of a mini-protocol.
type EgressClass int

const (
	// EgressClassPraos covers every mini-protocol other than the Leios
	// ones: ChainSync, BlockFetch, keep-alive and the rest.
	EgressClassPraos EgressClass = iota
	// EgressClassLeios covers the Leios mini-protocols (notify, fetch and
	// votes), whose bulk traffic must not delay Praos.
	EgressClassLeios
)

// The Leios mini-protocol numbers. They are repeated here because the
// protocol packages import this one; a test keeps them equal to the
// protocol packages' own constants.
const (
	protocolIdLeiosNotify uint16 = 18
	protocolIdLeiosFetch  uint16 = 19
	protocolIdLeiosVotes  uint16 = 20
)

// EgressClassOf returns the outbound traffic class of a mini-protocol.
func EgressClassOf(protocolId uint16) EgressClass {
	switch protocolId {
	case protocolIdLeiosNotify, protocolIdLeiosFetch, protocolIdLeiosVotes:
		return EgressClassLeios
	default:
		return EgressClassPraos
	}
}

const (
	// praosBurst is how many Praos segments may be written in a row while a
	// Leios segment waits, after which one Leios segment goes next. It bounds
	// how long Leios can be starved, at the cost of a waiting Praos segment
	// sometimes also waiting for one Leios write.
	praosBurst = 8
)

// EgressMetrics is an optional extension of Metrics. A Metrics value
// implementing it is also told about outbound scheduling.
type EgressMetrics interface {
	// EgressWait reports how long a segment waited for its turn to write
	// to the connection. It is called only for segments that had to wait.
	EgressWait(protocolId uint16, class EgressClass, d time.Duration)
}

// egress serializes writes to the connection. Its holder is the one
// goroutine writing; waiting segments are granted the turn Praos first, each
// class in arrival order. After praosBurst Praos grants in a row a waiting
// Leios segment goes next.
type egress struct {
	mu   sync.Mutex
	busy bool
	// Each waiter is a channel that is closed when it is granted the turn.
	praos    []chan struct{}
	leios    []chan struct{}
	praosRun int
}

// acquire waits for the segment's turn to write, reporting whether it had to
// wait. A nil error means the caller must call release when done.
func (e *egress) acquire(s *Segment, done <-chan bool) (bool, error) {
	granted := make(chan struct{})
	e.mu.Lock()
	if !e.busy {
		e.busy = true
		e.mu.Unlock()
		return false, nil
	}
	if EgressClassOf(s.GetProtocolId()) == EgressClassLeios {
		e.leios = append(e.leios, granted)
	} else {
		e.praos = append(e.praos, granted)
	}
	e.mu.Unlock()
	select {
	case <-granted:
		return true, nil
	case <-done:
	}
	e.mu.Lock()
	queued := e.remove(granted)
	e.mu.Unlock()
	if !queued {
		// The turn was decided as the muxer stopped; hand on a granted one.
		<-granted
		e.release()
	}
	return true, errors.New("shutting down")
}

// remove takes w out of its queue, reporting whether it was still there. The
// caller must hold mu.
func (e *egress) remove(w chan struct{}) bool {
	for _, q := range []*[]chan struct{}{&e.praos, &e.leios} {
		for i, c := range *q {
			if c == w {
				*q = append((*q)[:i], (*q)[i+1:]...)
				return true
			}
		}
	}
	return false
}

// release hands the turn to the next waiter, or frees it.
func (e *egress) release() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.leios) == 0 && len(e.praos) == 0 {
		e.busy = false
		e.praosRun = 0
		return
	}
	var w chan struct{}
	if len(e.leios) > 0 &&
		(len(e.praos) == 0 || e.praosRun >= praosBurst) {
		w = e.leios[0]
		e.leios = e.leios[1:]
		e.praosRun = 0
	} else {
		w = e.praos[0]
		e.praos = e.praos[1:]
		if len(e.leios) > 0 {
			e.praosRun++
		}
	}
	close(w)
}
