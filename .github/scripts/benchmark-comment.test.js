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

'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const {
  MAX_COMMENT_LENGTH,
  buildBenchmarkComment,
} = require('./benchmark-comment.js');

const metadata = {
  commitSha: '0123456789abcdef',
  triggeredBy: 'benchmark-author',
};

test('renders control sequences as visible text', () => {
  const body = buildBenchmarkComment({
    ...metadata,
    results: '\u001b[31mred\u001b[0m\n\u001b]8;;https://example.com\u0007link',
  });

  assert.match(body, /\\u\{1b\}\[31mred\\u\{1b\}\[0m/);
  assert.match(body, /\\u\{1b\}\]8;;https:\/\/example\.com\\u\{7\}link/);
  assert.doesNotMatch(body, /\u001b|\u0007/);
});

test('keeps arbitrary output inside inert code markup', () => {
  const body = buildBenchmarkComment({
    ...metadata,
    results: '</code></pre>\n```\n@everyone @octocat <script>',
  });

  assert.match(
    body,
    /&lt;\/code&gt;&lt;\/pre&gt;\n```\n@\u200beveryone @\u200boctocat &lt;script&gt;/,
  );
  assert.equal((body.match(/<\/code><\/pre>/g) || []).length, 1);
  assert.doesNotMatch(body, /@everyone|@octocat/);
});

test('preserves ordinary benchmark output', () => {
  const results = 'BenchmarkHash-8\t1234\t100 ns/op\n';
  const body = buildBenchmarkComment({ ...metadata, results });

  assert.match(body, new RegExp(results.replace('/', '\\/')));
  assert.match(body, /Triggered by: benchmark-author/);
});

test('caps the complete posted comment', () => {
  const body = buildBenchmarkComment({
    ...metadata,
    results: '&'.repeat(MAX_COMMENT_LENGTH),
  });

  assert.ok(body.length <= MAX_COMMENT_LENGTH);
  assert.match(body, /\.\.\. output truncated<\/code><\/pre>/);
});
