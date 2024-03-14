// Copyright 2023 Blink Labs Software
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

// Package utils provides random utility functions
package utils

import (
	"sync"
)

// DoneSignal provides a thread-safe way to close a channel and allows other routines to listen to the channel
type DoneSignal struct {
	closeCh chan struct{}
	once    sync.Once
}

func NewDoneSignal() *DoneSignal {
	return &DoneSignal{
		closeCh: make(chan struct{}),
	}
}

func (cn *DoneSignal) Close() {
	cn.once.Do(func() {
		close(cn.closeCh)
	})
}

func (cn *DoneSignal) GetCh() <-chan struct{} {
	return cn.closeCh
}
