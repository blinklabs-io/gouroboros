#!/usr/bin/env python3
"""Validate the subject and body supplied to a squash merge."""

import argparse
import re
import sys
from pathlib import Path

CONVENTIONAL = re.compile(
    r"^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)"
    r"(\([^()\n]+\))?!?: .+"
)
SIGNED_OFF = re.compile(r"^Signed-off-by: [^<>\n]+ <[^<>\n]+>$", re.M)
GENERIC = {"merge approved changes", "merge pr", "squash merge", "approved changes"}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--pr-title", required=True)
    parser.add_argument("--subject", required=True)
    parser.add_argument("--body-file", required=True, type=Path)
    args = parser.parse_args()

    errors = []
    if args.subject != args.pr_title:
        errors.append("subject must exactly match the PR title")
    if args.subject.casefold() in GENERIC:
        errors.append("generic merge subjects are not allowed")
    if not CONVENTIONAL.fullmatch(args.subject):
        errors.append("subject must be a Conventional Commit")
    if len(args.subject) > 72:
        errors.append("subject must be 72 characters or fewer")
    try:
        body = args.body_file.read_text(encoding="utf-8")
    except OSError as exc:
        errors.append(f"cannot read body file: {exc}")
    else:
        if "\\n" in body:
            errors.append("body contains literal escaped newlines")
        if not SIGNED_OFF.search(body):
            errors.append("body must contain a standalone Signed-off-by trailer")

    if errors:
        for error in errors:
            print(f"FAIL: {error}", file=sys.stderr)
        return 1
    print("ok: squash merge metadata is valid")
    return 0


if __name__ == "__main__":
    sys.exit(main())
