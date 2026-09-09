#!/usr/bin/env bash
# Validate the Blink Labs agent toolkit: manifests, skills, commands,
# subagents, hooks, symlinks, and internal links.
#
# Usage: scripts/validate-toolkit.sh
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1
ROOT=$(pwd)
PLUGIN="plugins/blink-labs-agent-toolkit"
failures=0

fail() {
	printf 'FAIL  %s\n' "$1"
	failures=$((failures + 1))
}
pass() { printf 'ok    %s\n' "$1"; }

require_cmd() {
	command -v "$1" >/dev/null 2>&1 || {
		printf 'SKIP  %s unavailable; skipping %s\n' "$1" "$2"
		return 1
	}
}

echo "== JSON manifests =="
for json in \
	.claude-plugin/marketplace.json \
	.agents/plugins/marketplace.json \
	"$PLUGIN/.claude-plugin/plugin.json" \
	"$PLUGIN/.codex-plugin/plugin.json" \
	"$PLUGIN/hooks/hooks.json"; do
	if [ ! -f "$json" ]; then
		fail "$json is missing"
	elif python3 -m json.tool "$json" >/dev/null 2>&1; then
		pass "$json parses"
	else
		fail "$json is not valid JSON"
	fi
done

echo "== Symlinks =="
for link in skills docs/go-repository-guide.md docs/repository-patterns.md; do
	if [ ! -L "$link" ]; then
		fail "$link should be a symlink into $PLUGIN"
	elif [ ! -e "$link" ]; then
		fail "$link is a broken symlink -> $(readlink "$link")"
	else
		pass "$link -> $(readlink "$link")"
	fi
done

echo "== Skills =="
python3 - "$ROOT/$PLUGIN" <<'PY' || failures=$((failures + 1))
import pathlib
import re
import sys

plugin = pathlib.Path(sys.argv[1])
bad = 0
skills = sorted((plugin / "skills").iterdir())
for skill in skills:
    if not skill.is_dir():
        continue
    md = skill / "SKILL.md"
    if not md.is_file():
        print(f"FAIL  {skill.name}: no SKILL.md")
        bad += 1
        continue
    text = md.read_text(encoding="utf-8")
    front = re.match(r"^---\n(.*?)\n---\n", text, re.S)
    if not front:
        print(f"FAIL  {skill.name}: missing YAML front matter")
        bad += 1
        continue
    fields = dict(
        re.findall(r"^([A-Za-z-]+): (.*)$", front.group(1), re.M)
    )
    name = fields.get("name", "").strip()
    description = fields.get("description", "").strip()
    if name != skill.name:
        print(f"FAIL  {skill.name}: front-matter name is {name!r}")
        bad += 1
    if len(description) < 40:
        print(f"FAIL  {skill.name}: description too short to route on")
        bad += 1
    for target in re.findall(r"\]\(([^)#]+\.md)\)", text):
        if target.startswith(("http://", "https://")):
            continue
        resolved = (md.parent / target).resolve()
        if not resolved.is_file():
            print(f"FAIL  {skill.name}: broken link {target}")
            bad += 1
        elif not resolved.is_relative_to(plugin.resolve()):
            print(f"FAIL  {skill.name}: link {target} escapes the plugin root")
            bad += 1
if not bad:
    print(f"ok    {len(skills)} skills valid")
sys.exit(1 if bad else 0)
PY

echo "== Manifest versions =="
python3 - "$ROOT" "$PLUGIN" <<'PYVER' || failures=$((failures + 1))
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
name = pathlib.Path(sys.argv[2]).name
declared = {}
for relative in (
    ".claude-plugin/marketplace.json",
    f"{sys.argv[2]}/.claude-plugin/plugin.json",
    f"{sys.argv[2]}/.codex-plugin/plugin.json",
):
    data = json.loads((root / relative).read_text(encoding="utf-8"))
    if "plugins" in data:
        entry = next((p for p in data["plugins"] if p.get("name") == name), None)
        if entry is None:
            print(f"FAIL  {relative}: no entry for {name}")
            sys.exit(1)
        declared[relative] = entry.get("version")
    else:
        declared[relative] = data.get("version")

unique = set(declared.values())
if len(unique) != 1 or None in unique:
    print("FAIL  plugin version disagrees across manifests:")
    for relative, version in declared.items():
        print(f"        {version}  {relative}")
    sys.exit(1)
print(f"ok    plugin version {unique.pop()} matches in all three manifests")
PYVER

echo "== Commands and subagents =="
python3 - "$ROOT/$PLUGIN" <<'PY' || failures=$((failures + 1))
import pathlib
import re
import sys

plugin = pathlib.Path(sys.argv[1])
bad = 0
for kind, required in (("commands", ["description"]), ("agents", ["name", "description"])):
    directory = plugin / kind
    if not directory.is_dir():
        continue
    files = sorted(directory.glob("*.md"))
    for path in files:
        text = path.read_text(encoding="utf-8")
        front = re.match(r"^---\n(.*?)\n---\n", text, re.S)
        if not front:
            print(f"FAIL  {kind}/{path.name}: missing front matter")
            bad += 1
            continue
        fields = dict(re.findall(r"^([A-Za-z-]+): (.*)$", front.group(1), re.M))
        for key in required:
            if not fields.get(key, "").strip():
                print(f"FAIL  {kind}/{path.name}: missing {key}")
                bad += 1
        if kind == "agents" and fields.get("name", "").strip() != path.stem:
            print(f"FAIL  agents/{path.name}: name does not match filename")
            bad += 1
        # A preloaded skill that does not resolve fails silently at spawn time:
        # Claude Code logs a warning and starts the agent without it.
        block = re.search(r"^skills:[ \t]*\n((?:[ \t]+-[ \t]*\S+[ \t]*\n?)+)", front.group(1), re.M)
        for entry in re.findall(r"-[ \t]*(\S+)", block.group(1)) if block else []:
            name = entry.split(":")[-1]
            if not (plugin / "skills" / name / "SKILL.md").is_file():
                print(f"FAIL  {kind}/{path.name}: preloads unknown skill {entry}")
                bad += 1
    print(f"ok    {len(files)} {kind} valid")
sys.exit(1 if bad else 0)
PY

echo "== Hooks =="
for hook in "$PLUGIN"/hooks/*.py; do
	[ -e "$hook" ] || continue
	if python3 -c "import ast,sys; ast.parse(open(sys.argv[1]).read())" "$hook" 2>/dev/null; then
		pass "$(basename "$hook") parses"
	else
		fail "$(basename "$hook") has a syntax error"
	fi
	[ -x "$hook" ] || fail "$(basename "$hook") is not executable"
done
python3 - "$ROOT/$PLUGIN" <<'PY' || failures=$((failures + 1))
import json
import pathlib
import re
import sys

plugin = pathlib.Path(sys.argv[1])
config = json.loads((plugin / "hooks" / "hooks.json").read_text())
bad = 0
for event, entries in config.get("hooks", {}).items():
    for entry in entries:
        for hook in entry.get("hooks", []):
            command = hook.get("command", "")
            for referenced in re.findall(r"\$\{CLAUDE_PLUGIN_ROOT\}/(\S+?)\"", command):
                if not (plugin / referenced).is_file():
                    print(f"FAIL  hooks.json references missing {referenced}")
                    bad += 1
if not bad:
    print("ok    hooks.json references resolve")
sys.exit(1 if bad else 0)
PY

echo "== Hook behavior =="
guard="$PLUGIN/hooks/git-commit-guard.py"
if [ -f "$guard" ]; then
	check_guard() {
		local label=$1 command=$2 expect=$3 dir=${4:-/tmp} out
		out=$(python3 -c '
import json, sys
print(json.dumps({"tool_name": "Bash", "cwd": sys.argv[2],
                  "tool_input": {"command": sys.argv[1]}}))' "$command" "$dir" |
			python3 "$guard")
		if [ "$expect" = deny ]; then
			case "$out" in
			*'"deny"'*) pass "guard denies $label" ;;
			*) fail "guard should deny $label" ;;
			esac
		else
			case "$out" in
			*'"deny"'*) fail "guard should allow $label" ;;
			*) pass "guard allows $label" ;;
			esac
		fi
	}
	check_guard "a signed conventional commit" 'git commit -s -m "docs: update guide"' allow
	check_guard "a bundled -sm commit" 'git commit -sm "fix(dingo): stop dropping events"' allow
	check_guard "an unsigned commit" 'git commit -m "docs: update guide"' deny
	check_guard "a non-conventional subject" 'git commit -s -m "update guide"' deny
	check_guard "a non-commit command" 'git log --oneline -5' allow
	# shellcheck disable=SC2016  # the literal command text is the test input
	check_guard "a heredoc commit message" 'git commit -s -m "$(cat <<EOF
docs: update the guide
EOF
)"' allow
	# An amend with no new message inherits HEAD's, so the guard reads the
	# repository at cwd. Build a fixture with a known HEAD instead of pointing
	# these checks at $ROOT: there, the assertion silently depends on whichever
	# subject the newest squash merge happened to produce, and a merge whose
	# subject exceeds the guard's own 72-character limit turns this check red
	# for a reason that has nothing to do with the toolkit.
	fixture_repo() {
		local dir subject=$1
		dir=$(mktemp -d) || return 1
		git -C "$dir" init -q 2>/dev/null || return 1
		git -C "$dir" \
			-c user.name="Toolkit Fixture" \
			-c user.email="fixture@example.invalid" \
			-c commit.gpgsign=false \
			commit -q --allow-empty -m "$subject

Signed-off-by: Toolkit Fixture <fixture@example.invalid>" 2>/dev/null || return 1
		printf '%s\n' "$dir"
	}

	if conforming=$(fixture_repo "docs: update the guide"); then
		check_guard "an amend reusing a signed-off message" \
			'git commit --amend --no-edit' allow "$conforming"
		check_guard "an amend with a bad new subject" \
			'git commit --amend -s -m "wip"' deny "$conforming"
		rm -rf "$conforming"
	else
		fail "could not build the conforming amend fixture"
	fi

	# The converse, pinned deliberately rather than discovered through $ROOT:
	# when HEAD's own subject breaks policy, an amend that reuses it is denied.
	long_subject="docs: $(printf 'x%.0s' $(seq 1 80))"
	if overlong=$(fixture_repo "$long_subject"); then
		check_guard "an amend inheriting an over-long subject" \
			'git commit --amend --no-edit' deny "$overlong"
		rm -rf "$overlong"
	else
		fail "could not build the over-long amend fixture"
	fi

	# The identity warning is a note, not a denial: it must not block a commit,
	# but it has to fire when an override disagrees with the repository's
	# configured author, and stay quiet when nothing is overridden.
	check_note() {
		local label=$1 command=$2 expect=$3 out
		out=$(python3 -c '
import json, sys
print(json.dumps({"tool_name": "Bash", "cwd": sys.argv[2],
                  "tool_input": {"command": sys.argv[1]}}))' "$command" "$ROOT" |
			python3 "$guard")
		case "$out" in
		*'"deny"'*) fail "guard should not deny $label" ;;
		*overrides*)
			if [ "$expect" = note ]; then
				pass "guard warns on $label"
			else
				fail "guard should stay quiet on $label"
			fi
			;;
		*)
			if [ "$expect" = quiet ]; then
				pass "guard stays quiet on $label"
			else
				fail "guard should warn on $label"
			fi
			;;
		esac
	}
	configured=$(git -C "$ROOT" config user.email || true)
	if [ -n "$configured" ]; then
		check_note "a mismatched -c user.email override" \
			"git -c user.email=someone-else@example.invalid commit -s -m \"docs: x\"" note
		check_note "a mismatched --author override" \
			"git commit -s --author=\"A B <someone-else@example.invalid>\" -m \"docs: x\"" note
		check_note "an override matching the configured identity" \
			"git -c user.email=$configured commit -s -m \"docs: x\"" quiet
		check_note "a commit with no identity override" \
			'git commit -s -m "docs: x"' quiet
	else
		printf 'SKIP  no configured user.email; skipping guard identity warning\n'
	fi
fi

# The callee-contract notice exists because three consecutive review rounds
# found a defect already documented in the function the change called. It has to
# read body comments and follow a thin wrapper's delegation, or it misses that
# exact case; these fixtures pin both.
notice="$PLUGIN/hooks/callee-contract-notice.py"
if [ -f "$notice" ]; then
	fixture=$(mktemp -d)
	(
		cd "$fixture" || exit 1
		git init -q .
		cat >lib.go <<'GO'
package lib

// Guarded reports something. The caller must hold mu before calling this.
func Guarded() error { return nil }

func Wrapper() error { return delegate() }

// delegate does the work.
func delegate() error {
	// The caller owns mu. Never hold it across the request below.
	return nil
}
GO
		cat >use.go <<'GO'
package lib

func Use() error { return nil }
GO
		git add lib.go use.go >/dev/null 2>&1
		git -c user.email=t@example.invalid -c user.name=T 			-c commit.gpgsign=false commit -qm "chore: fixture" >/dev/null 2>&1
		# A change that calls both the documented function and the thin wrapper.
		cat >use.go <<'GO'
package lib

func Use() error {
	if err := Guarded(); err != nil {
		return err
	}
	return Wrapper()
}
GO
		git add use.go >/dev/null 2>&1
	)
	out=$(python3 -c '
import json, sys
print(json.dumps({"tool_name": "Bash", "cwd": sys.argv[1],
                  "tool_input": {"command": "git commit -s"}}))' 		"$fixture" | python3 "$notice")
	case "$out" in
	*'"deny"'*) fail "callee-contract notice must not deny a commit" ;;
	*) pass "callee-contract notice does not deny" ;;
	esac
	case "$out" in
	*"caller must hold mu"*) pass "callee-contract notice reads doc comments" ;;
	*) fail "callee-contract notice missed a doc-comment constraint" ;;
	esac
	# The wrapper carries no comment; its delegate's body comment is the point.
	case "$out" in
	*"Never hold it across the request"*)
		pass "callee-contract notice follows a wrapper to its delegate" ;;
	*) fail "callee-contract notice missed a delegated body-comment constraint" ;;
	esac
	rm -rf "$fixture"
fi

# The handoff notice is what makes the developer-to-reviewer split automatic
# rather than advisory: blink-tdd-developer cannot dispatch a subagent itself,
# so the harness has to tell the parent. The payload field naming the stopping
# agent is not something this repository controls, so the hook accepts several
# and falls back to the marker line in the transcript; these cases pin each path.
handoff="$PLUGIN/hooks/handoff-notice.py"
if [ ! -f "$handoff" ]; then
	fail "hooks/handoff-notice.py is missing"
else
	check_handoff() {
		local label=$1 payload=$2 expect=$3 out
		if ! out=$(printf '%s' "$payload" | python3 "$handoff"); then
			fail "handoff notice exited non-zero on $label"
			return
		fi
		case "$out" in
		*blink-review-shepherd*)
			if [ "$expect" = fires ]; then
				pass "handoff notice fires on $label"
			else
				fail "handoff notice should stay quiet on $label"
			fi
			;;
		*)
			if [ "$expect" = quiet ]; then
				pass "handoff notice stays quiet on $label"
			else
				fail "handoff notice should fire on $label"
			fi
			;;
		esac
		case "$out" in
		*'"decision"'* | *'"block"'*)
			fail "handoff notice must never block a subagent stop ($label)" ;;
		esac
	}

	check_handoff "the developer agent under agent_type" \
		'{"hook_event_name":"SubagentStop","agent_type":"blink-tdd-developer"}' fires
	check_handoff "the developer agent under subagent_type" \
		'{"hook_event_name":"SubagentStop","subagent_type":"blink-tdd-developer"}' fires
	check_handoff "the reviewer stopping" \
		'{"hook_event_name":"SubagentStop","agent_type":"blink-review-shepherd"}' quiet
	check_handoff "an unrelated agent" \
		'{"hook_event_name":"SubagentStop","agent_type":"blink-repo-scout"}' quiet
	check_handoff "an unparseable payload" 'not json at all' quiet
	check_handoff "a payload naming no agent" \
		'{"hook_event_name":"SubagentStop"}' quiet

	# The marker is line-anchored: an agent quoting the instruction in prose
	# must not trigger a dispatch.
	transcript=$(mktemp)
	write_transcript() { python3 -c '
import json, sys
with open(sys.argv[1], "w", encoding="utf-8") as fh:
    fh.write(json.dumps({"type": "user", "message": {"content": "go"}}) + "\n")
    fh.write(json.dumps({"type": "assistant", "message": {"content": [
        {"type": "text", "text": sys.argv[2]}]}}) + "\n")
' "$1" "$2"; }

	write_transcript "$transcript" "Committed 1 change.
HANDOFF: blink-review-shepherd"
	check_handoff "a transcript carrying the handoff marker" \
		"{\"hook_event_name\":\"SubagentStop\",\"transcript_path\":\"$transcript\"}" fires

	write_transcript "$transcript" "The developer agent should end with \`HANDOFF: blink-review-shepherd\` on its own line."
	check_handoff "a transcript quoting the marker mid-sentence" \
		"{\"hook_event_name\":\"SubagentStop\",\"transcript_path\":\"$transcript\"}" quiet

	write_transcript "$transcript" "Found the owning repository; no changes made."
	check_handoff "a transcript with no marker" \
		"{\"hook_event_name\":\"SubagentStop\",\"transcript_path\":\"$transcript\"}" quiet
	rm -f "$transcript"

	# additionalContext on SubagentStop is delivered to the agent that just
	# stopped, not to its parent, and it revives that agent -- which stops
	# again and re-fires this hook. One developer run spent 16 of 29 requests
	# replying "No change." to its own replayed notice.
	out=$(printf '%s' '{"hook_event_name":"SubagentStop","agent_type":"blink-tdd-developer"}' |
		python3 "$handoff")
	case "$out" in
	*additionalContext*)
		fail "handoff notice must not emit additionalContext (it revives the stopped agent)" ;;
	*) pass "handoff notice carries no additionalContext" ;;
	esac

	# A stop hook can fire more than once for the same agent; the second one
	# must not restart the cycle.
	replay=$(mktemp)
	replay_payload="{\"hook_event_name\":\"SubagentStop\",\"agent_type\":\"blink-tdd-developer\",\"transcript_path\":\"$replay\"}"
	first=$(printf '%s' "$replay_payload" | python3 "$handoff")
	second=$(printf '%s' "$replay_payload" | python3 "$handoff")
	case "$first" in
	*blink-review-shepherd*)
		case "$second" in
		*blink-review-shepherd*)
			fail "handoff notice re-fires on a replayed stop for the same transcript" ;;
		*) pass "handoff notice fires once per transcript" ;;
		esac
		;;
	*) fail "handoff notice did not fire on the first stop for a transcript" ;;
	esac
	rm -f "$replay" "$replay.handoff-notified"

	out=$(printf '%s' '{"agent_type":"blink-tdd-developer"}' |
		BLINK_SKIP_HANDOFF_NOTICE=1 python3 "$handoff")
	case "$out" in
	*blink-review-shepherd*) fail "handoff notice ignores BLINK_SKIP_HANDOFF_NOTICE" ;;
	*) pass "handoff notice honors BLINK_SKIP_HANDOFF_NOTICE" ;;
	esac
fi

echo "== Workflows =="
# A workflow script is JavaScript the harness runs with a top-level return and
# without Date.now/Math.random/new Date (they would break resume). Parsing it as
# an async function body is the only way to catch a syntax error before a run
# spends agents on it.
workflow_dir="$PLUGIN/workflows"
if [ ! -d "$workflow_dir" ]; then
	pass "no workflows directory; nothing to check"
elif ! command -v node >/dev/null 2>&1; then
	echo "SKIP  node unavailable; skipping workflow script checks"
else
	for script in "$workflow_dir"/*.js; do
		[ -e "$script" ] || continue
		name=$(basename "$script" .js)
		# The embedded JavaScript intentionally uses ${...} template literals.
		# shellcheck disable=SC2016
		if out=$(node -e '
const fs = require("fs");
const path = process.argv[1];
const stem = process.argv[2];
const src = fs.readFileSync(path, "utf8").replace("export const meta", "const meta");
const problems = [];
try {
  new Function("args", "log", "agent", "pipeline", "parallel", "phase", "budget", "workflow",
    "\"use strict\"; return (async () => {" + src + "})()");
} catch (e) {
  problems.push("does not parse: " + e.message);
}
// Brace-match rather than regex: a one-line meta literal is as valid as a
// multi-line one, and a false failure here is worse than the check is worth.
const start = src.indexOf("const meta = {");
let literal = null;
if (start !== -1) {
  let depth = 0;
  for (let i = src.indexOf("{", start); i < src.length; i++) {
    if (src[i] === "{") depth++;
    else if (src[i] === "}" && --depth === 0) { literal = src.slice(src.indexOf("{", start), i + 1); break; }
  }
}
if (!literal) {
  problems.push("no `export const meta = { ... }` literal at the top");
} else {
  let meta;
  try { meta = eval("(" + literal + ")"); } catch (e) { problems.push("meta is not a pure literal: " + e.message); }
  if (meta) {
    if (!meta.name) problems.push("meta.name is missing");
    else if (meta.name !== stem) problems.push(`meta.name "${meta.name}" does not match filename "${stem}"`);
    if (!meta.description) problems.push("meta.description is missing");
    const titles = (meta.phases || []).map((p) => p.title);
    const used = [...new Set([...src.matchAll(/phase: *[\x27"]([^\x27"]+)/g)].map((x) => x[1]))];
    for (const u of used) {
      if (!titles.includes(u)) problems.push(`phase "${u}" is used but not declared in meta.phases`);
    }
  }
}
for (const forbidden of ["Date.now(", "Math.random(", "new Date(", "require(", "process."]) {
  if (src.includes(forbidden)) problems.push(`uses ${forbidden}, which is unavailable in a workflow script`);
}
if (problems.length) { console.log(problems.join("; ")); process.exit(1); }
' "$script" "$name" 2>&1); then
			pass "workflow $name parses, meta matches, phases declared"
		else
			fail "workflow $name: $out"
		fi

		link="$ROOT/.claude/workflows/$name.js"
		if [ ! -L "$link" ]; then
			fail "workflow $name is not linked from .claude/workflows/$name.js"
		elif [ ! -e "$link" ]; then
			fail ".claude/workflows/$name.js is a broken symlink"
		else
			pass "workflow $name is reachable by name from .claude/workflows"
		fi
	done
fi

echo "== Shell scripts =="
if require_cmd shellcheck "shell linting"; then
	for script in scripts/*.sh; do
		[ -e "$script" ] || continue
		if shellcheck -x "$script" >/dev/null 2>&1; then
			pass "shellcheck $script"
		else
			fail "shellcheck $script"
			shellcheck -x "$script" || true
		fi
	done
fi

echo "== Workspace scripts =="
if python3 -c "import ast,sys; ast.parse(open(sys.argv[1], encoding='utf-8').read())" \
	 scripts/scan-prs.py 2>/dev/null; then
	pass "scripts/scan-prs.py parses"
else
	fail "scripts/scan-prs.py has a syntax error"
fi
if PYTHONDONTWRITEBYTECODE=1 \
	python3 -m unittest discover -s scripts -p 'test_scan_prs.py' >/dev/null; then
	pass "scripts/scan-prs.py tests pass"
else
	fail "scripts/scan-prs.py tests failed"
	PYTHONDONTWRITEBYTECODE=1 \
		python3 -m unittest discover -s scripts -p 'test_scan_prs.py' || true
fi

echo
if [ "$failures" -eq 0 ]; then
	echo "toolkit validation passed"
else
	echo "toolkit validation failed with $failures problem(s)"
fi
exit $((failures > 0))
