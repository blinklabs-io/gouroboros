#!/usr/bin/env python3
"""Scan open GitHub pull requests for review and merge-gate state.

The scan deliberately uses GitHub's REST API through ``gh`` so it can inspect
review records and checks for the exact current head SHA. It never writes to
GitHub.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import Any


BOT_USERS = {"coderabbitai[bot]", "cubic-dev-ai[bot]"}

# The REST API suffixes bot logins with "[bot]" but GraphQL does not, and some
# integrations never carry the suffix, so a suffix test alone counts them as
# people. Match on both.
BOT_LOGINS = {
    "coderabbitai",
    "cubic-dev-ai",
    "github-advanced-security",
    "dependabot",
    "sg-doc-holiday",
    "codecov",
    "sonarcloud",
}
PASS_CONCLUSIONS = {"success", "neutral", "skipped"}


def gh_json(args: list[str]) -> Any:
    result = subprocess.run(
        ["gh", *args],
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode:
        detail = result.stderr.strip() or result.stdout.strip()
        raise RuntimeError(f"gh {' '.join(args)}: {detail}")
    return json.loads(result.stdout)


def review_summary(body: str) -> str:
    for line in body.splitlines():
        line = line.strip()
        if re.search(
            r"\*\*.*(?:issues found|Actionable comments posted|No issues found|"
            r"All reported issues were addressed)",
            line,
            re.IGNORECASE,
        ):
            return re.sub(r"<[^>]+>", "", line)
    return ""


def comment_excerpt(body: str, limit: int = 140) -> str:
    """Return a one-line excerpt of a comment body for the report."""
    text = re.sub(r"<[^>]+>", "", body)
    text = " ".join(text.split())
    return text[: limit - 1] + "\u2026" if len(text) > limit else text


def bot_has_actionable_findings(body: str) -> bool:
    if re.search(
        r"\*\*(?:No issues found|All reported issues were addressed)",
        body,
        re.IGNORECASE,
    ):
        return False
    if re.search(r"\*\*Actionable comments posted:\s*[1-9]\d*", body):
        return True
    if re.search(r"\*\*[1-9]\d* issues? found\*\*", body):
        return True
    return bool(re.search(r"unresolved issues.*violation|P[123]:", body, re.IGNORECASE))


def latest_reviews(reviews: list[dict[str, Any]], head: str) -> dict[str, dict[str, Any]]:
    latest: dict[str, dict[str, Any]] = {}
    for review in reviews:
        user = (review.get("user") or {}).get("login", "unknown")
        if not latest.get(user) or review.get("submitted_at", "") > latest[user].get(
            "submitted_at", ""
        ):
            latest[user] = review

    current: dict[str, dict[str, Any]] = {}
    for user, review in latest.items():
        if review.get("commit_id") != head:
            continue
        body = review.get("body") or ""
        current[user] = {
            "state": review.get("state", ""),
            "submitted_at": review.get("submitted_at", ""),
            "summary": review_summary(body),
            "actionable": user in BOT_USERS and bot_has_actionable_findings(body),
        }
    return current


def inspect_pr(pr: dict[str, Any]) -> dict[str, Any]:
    repo = pr["repository"]["nameWithOwner"]
    number = pr["number"]
    prefix = f"repos/{repo}"
    try:
        metadata = gh_json(["api", f"{prefix}/pulls/{number}"])
        head = metadata["head"]["sha"]
        reviews = gh_json(["api", f"{prefix}/pulls/{number}/reviews?per_page=100"])
        # Plain PR comments are a separate surface from reviews and inline
        # threads. A reviewer can request changes in an ordinary comment, which
        # produces no review record and no thread, so a scan that reads only
        # reviews reports the PR as unreviewed.
        issue_comments = gh_json(
            ["api", f"{prefix}/issues/{number}/comments?per_page=100"]
        )
        checks = gh_json(
            [
                "api",
                f"{prefix}/commits/{head}/check-runs?per_page=100",
            ]
        ).get("check_runs", [])
        current_reviews = latest_reviews(reviews, head)
        # Anchored on the author's own last comment, not on the head commit: a
        # push is not evidence that a reviewer's comment was answered, so a
        # head-relative cutoff silently drops feedback that predates it.
        pr_author = pr["author"]["login"]
        author_replies = [
            comment.get("created_at", "")
            for comment in issue_comments
            if (comment.get("user") or {}).get("login") == pr_author
        ]
        last_author_reply = max(author_replies) if author_replies else ""
        # Human reviews carrying a body, at any state. A reviewer can put
        # blocking feedback in a COMMENTED review, which the changes-requested
        # section below never shows.
        human_reviews = [
            {
                "user": (review.get("user") or {}).get("login", "unknown"),
                "state": review.get("state", ""),
                "submitted_at": review.get("submitted_at", ""),
                "on_head": review.get("commit_id") == head,
                "excerpt": comment_excerpt(review.get("body") or ""),
                "url": review.get("html_url", ""),
            }
            for review in reviews
            if is_human((review.get("user") or {}).get("login", ""))
            and (review.get("user") or {}).get("login") != pr_author
            and (review.get("body") or "").strip()
        ]
        human_reviews.sort(key=lambda item: item["submitted_at"])
        human_comments = [
            {
                "user": (comment.get("user") or {}).get("login", "unknown"),
                "created_at": comment.get("created_at", ""),
                "excerpt": comment_excerpt(comment.get("body") or ""),
                "url": comment.get("html_url", ""),
            }
            for comment in issue_comments
            if is_human((comment.get("user") or {}).get("login", ""))
        ]
        human_comments.sort(key=lambda item: item["created_at"])
        problem_checks = [
            {
                "name": check.get("name", ""),
                "status": check.get("status", ""),
                "conclusion": check.get("conclusion"),
                "details_url": check.get("details_url", ""),
            }
            for check in checks
            if check.get("status") != "completed"
            or str(check.get("conclusion", "")).lower() not in PASS_CONCLUSIONS
        ]
        return {
            "repository": repo,
            "number": number,
            "title": pr["title"],
            "url": pr["url"],
            "author": pr["author"]["login"],
            "updated_at": pr["updatedAt"],
            "head": head,
            "mergeable_state": metadata.get("mergeable_state", ""),
            "requested_reviewers": [
                reviewer.get("login", "")
                for reviewer in metadata.get("requested_reviewers", [])
            ],
            "requested_teams": [team.get("name", "") for team in metadata.get("requested_teams", [])],
            "current_reviews": current_reviews,
            "human_comments": human_comments,
            "human_reviews": human_reviews,
            "last_author_reply": last_author_reply,
            "problem_checks": problem_checks,
            "error": None,
        }
    except (KeyError, RuntimeError, json.JSONDecodeError) as error:
        return {
            "repository": repo,
            "number": number,
            "title": pr["title"],
            "url": pr["url"],
            "author": pr["author"]["login"],
            "updated_at": pr["updatedAt"],
            "error": str(error),
        }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--owner", default=os.environ.get("GITHUB_ORG", "blinklabs-io"))
    parser.add_argument("--user", default=None, help="login used for ownership/review-request summaries")
    parser.add_argument("--limit", type=int, default=200)
    parser.add_argument("--workers", type=int, default=8)
    parser.add_argument("--include-drafts", action="store_true")
    parser.add_argument("--include-dependabot", action="store_true")
    parser.add_argument("--format", choices=("text", "json"), default="text")
    return parser.parse_args()


def scan(args: argparse.Namespace) -> list[dict[str, Any]]:
    prs = gh_json(
        [
            "search",
            "prs",
            "--owner",
            args.owner,
            "--state",
            "open",
            "--limit",
            str(args.limit),
            "--json",
            "repository,number,title,author,isDraft,updatedAt,url",
        ]
    )
    prs = [
        pr
        for pr in prs
        if (args.include_drafts or not pr["isDraft"])
        and (args.include_dependabot or pr["author"]["login"] != "dependabot[bot]")
    ]
    results: list[dict[str, Any]] = []
    with ThreadPoolExecutor(max_workers=max(1, args.workers)) as pool:
        futures = {pool.submit(inspect_pr, pr): pr for pr in prs}
        for future in as_completed(futures):
            results.append(future.result())
    return sorted(results, key=lambda item: (item["repository"], item["number"]))


def is_human(user: str) -> bool:
    return bool(user) and not user.endswith("[bot]") and user not in BOT_LOGINS


def print_text(results: list[dict[str, Any]], user: str | None) -> None:
    owned = [item for item in results if user and item.get("author") == user]
    bot_findings = [
        (item, reviewer, review)
        for item in results
        for reviewer, review in item.get("current_reviews", {}).items()
        if review.get("actionable")
    ]
    failures = [item for item in results if item.get("problem_checks")]
    changes = [
        (item, reviewer)
        for item in results
        for reviewer, review in item.get("current_reviews", {}).items()
        if is_human(reviewer) and review.get("state") == "CHANGES_REQUESTED"
    ]
    approvals = [
        (item, reviewer)
        for item in results
        for reviewer, review in item.get("current_reviews", {}).items()
        if is_human(reviewer) and review.get("state") == "APPROVED"
    ]
    requested = [
        item
        for item in results
        if user and user in item.get("requested_reviewers", [])
    ]
    # Human PR comments made after the current head was committed: feedback
    # that arrived since the last push and has no review record behind it.
    fresh_comments = [
        (item, comment)
        for item in results
        for comment in item.get("human_comments", [])
        # Anything the PR author has not answered yet, however old.
        if comment.get("created_at", "") > (item.get("last_author_reply") or "")
        and comment.get("user") != item.get("author")
    ]
    fresh_comments.sort(key=lambda pair: pair[1].get("created_at", ""))

    print(
        f"PR scan: {len(results)} open PRs "
        f"({len(owned)} authored by {user or 'selected user'})"
    )

    def section(title: str, lines: list[str]) -> None:
        print(f"\n{title}")
        print("\n".join(lines) if lines else "none")

    section(
        "CURRENT BOT FINDINGS",
        [
            f"- {item['repository']}#{item['number']} {bot}: {review.get('summary') or 'actionable review content'}"
            f"\n  {item['url']}"
            for item, bot, review in bot_findings
        ],
    )
    section(
        "HUMAN PR COMMENTS THE AUTHOR HAS NOT ANSWERED",
        [
            f"- {item['repository']}#{item['number']} {comment['user']} "
            f"({comment['created_at']}): {comment['excerpt']}"
            f"\n  {comment['url'] or item['url']}"
            for item, comment in fresh_comments
        ],
    )
    section(
        "HUMAN REVIEWS WITH FEEDBACK (ANY STATE)",
        [
            f"- {item['repository']}#{item['number']} {review['user']} "
            f"{review['state']}"
            + ("" if review["on_head"] else " (not on current head)")
            + f": {review['excerpt']}\n  {review['url'] or item['url']}"
            for item in results
            for review in item.get("human_reviews", [])
        ],
    )
    section(
        "MERGE CONFLICTS",
        [
            f"- {item['repository']}#{item['number']} ({state}): {item['title'][:60]}"
            f"\n  {item['url']}"
            for item in results
            for state in [item.get("mergeable_state", "")]
            # "dirty" means conflicts. "unknown" means GitHub has not finished
            # computing it, which is not the same as mergeable — report it so it
            # gets re-checked rather than silently passing.
            if state in ("dirty", "unknown")
        ],
    )
    section(
        "FAILING OR PENDING CHECKS",
        [
            f"- {item['repository']}#{item['number']}: "
            + ", ".join(
                f"{check['name']} ({check['conclusion'] or check['status']})"
                for check in item["problem_checks"]
            )
            + f"\n  {item['url']}"
            for item in failures
        ],
    )
    section(
        "CURRENT HUMAN CHANGES REQUESTED",
        [f"- {item['repository']}#{item['number']} by {reviewer}\n  {item['url']}" for item, reviewer in changes],
    )
    section(
        "CURRENT HUMAN APPROVALS",
        [
            f"- {item['repository']}#{item['number']} by {reviewer} ({item.get('mergeable_state', 'unknown')})\n  {item['url']}"
            for item, reviewer in approvals
        ],
    )
    section(
        f"DIRECT REVIEW REQUESTS FOR {user or 'SELECTED USER'}",
        [f"- {item['repository']}#{item['number']}\n  {item['url']}" for item in requested],
    )


def main() -> int:
    args = parse_args()
    try:
        user = args.user or gh_json(["api", "user"])["login"]
        results = scan(args)
    except (RuntimeError, json.JSONDecodeError, OSError) as error:
        print(f"scan failed: {error}", file=sys.stderr)
        return 1
    if args.format == "json":
        print(json.dumps({"user": user, "pull_requests": results}, indent=2))
    else:
        print_text(results, user)
    return 0 if not any(item.get("error") for item in results) else 1


if __name__ == "__main__":
    raise SystemExit(main())
