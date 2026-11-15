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

package consensus

import "errors"

// The shared fixtures module imports this package, so in-package tests cannot
// import it. An external test file installs the reader before tests run.
var embeddedFixtureReader func(name string) ([]byte, error)

func readEmbeddedFixture(name string) ([]byte, error) {
	if embeddedFixtureReader == nil {
		return nil, errors.New("embedded fixture reader not installed")
	}
	return embeddedFixtureReader(name)
}

// SetEmbeddedFixtureReader installs the embedded-fixture reader.
func SetEmbeddedFixtureReader(f func(name string) ([]byte, error)) {
	embeddedFixtureReader = f
}

type embeddedFixtureFS struct{}

func (embeddedFixtureFS) ReadFile(name string) ([]byte, error) {
	return readEmbeddedFixture(name)
}
