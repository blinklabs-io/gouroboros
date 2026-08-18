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
	check_guard "an amend reusing a signed-off message" \
		'git commit --amend --no-edit' allow "$ROOT"
	check_guard "an amend with a bad new subject" \
		'git commit --amend -s -m "wip"' deny "$ROOT"
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

echo
if [ "$failures" -eq 0 ]; then
	echo "toolkit validation passed"
else
	echo "toolkit validation failed with $failures problem(s)"
fi
exit $((failures > 0))
