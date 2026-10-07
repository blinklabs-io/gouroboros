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

const MAX_COMMENT_LENGTH = 60000;
const TRUNCATION_MARKER = '\n... output truncated';
const controlOrFormat = /[\p{Cc}\p{Cf}]/u;

function visibleChunk(character) {
  switch (character) {
    case '\n':
    case '\t':
      return character;
    case '&':
      return '&amp;';
    case '<':
      return '&lt;';
    case '>':
      return '&gt;';
    case '@':
      return '@\u200b';
    default:
      if (controlOrFormat.test(character)) {
        return `\\u{${character.codePointAt(0).toString(16)}}`;
      }
      return character;
  }
}

function escapeVisibleText(value, limit) {
  let text = '';
  for (const character of value) {
    const chunk = visibleChunk(character);
    if (text.length + chunk.length > limit) {
      return { text, truncated: true };
    }
    text += chunk;
  }
  return { text, truncated: false };
}

function buildBenchmarkComment({ results, commitSha, triggeredBy }) {
  const prefix = [
    '## Benchmark Comparison (base vs PR)',
    '',
    '<details>',
    '<summary>Click to expand</summary>',
    '',
    '<pre><code>',
  ].join('\n');
  const suffix = [
    '</code></pre>',
    '',
    '</details>',
    '',
    `Commit: ${commitSha}`,
    `Triggered by: ${triggeredBy}`,
  ].join('\n');
  const available = MAX_COMMENT_LENGTH - prefix.length - suffix.length;
  let escaped = escapeVisibleText(results, available);
  if (escaped.truncated) {
    escaped = escapeVisibleText(
      results,
      available - TRUNCATION_MARKER.length,
    );
    escaped.text += TRUNCATION_MARKER;
  }
  return `${prefix}${escaped.text}${suffix}`;
}

module.exports = {
  MAX_COMMENT_LENGTH,
  buildBenchmarkComment,
};
