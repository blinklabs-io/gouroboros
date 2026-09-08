#!/usr/bin/env python3
"""Focused tests for the workspace pull-request scanner."""

from __future__ import annotations

import contextlib
import importlib.util
import io
import os
import pathlib
import unittest
from unittest import mock


SCRIPT = pathlib.Path(
    os.environ.get("SCAN_PRS_PATH", pathlib.Path(__file__).with_name("scan-prs.py"))
)
SPEC = importlib.util.spec_from_file_location("scan_prs", SCRIPT)
assert SPEC and SPEC.loader
scan_prs = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(scan_prs)


def result(**overrides: object) -> dict[str, object]:
    item: dict[str, object] = {
        "repository": "blinklabs-io/example",
        "number": 17,
        "title": "Example",
        "url": "https://github.com/blinklabs-io/example/pull/17",
        "author": "author",
        "current_reviews": {},
        "human_comments": [],
        "human_reviews": [],
        "problem_checks": [],
        "requested_reviewers": [],
        "requested_team_slugs": [],
        "review_request_sources": [],
    }
    item.update(overrides)
    return item


class ScanPrsTests(unittest.TestCase):
    def test_team_membership_matches_requested_team(self) -> None:
        item = result(requested_team_slugs=["blinklabs-io/core"])

        sources = scan_prs.review_request_sources(
            item, "reviewer", ["blinklabs-io/core"]
        )

        self.assertEqual(sources, ["team:blinklabs-io/core"])

    def test_unrelated_team_does_not_match(self) -> None:
        item = result(requested_team_slugs=["blinklabs-io/docs"])

        sources = scan_prs.review_request_sources(
            item, "reviewer", ["blinklabs-io/core"]
        )

        self.assertEqual(sources, [])

    def test_direct_and_team_requests_keep_both_reasons(self) -> None:
        item = result(
            requested_reviewers=["Reviewer"],
            requested_team_slugs=["blinklabs-io/core"],
        )

        sources = scan_prs.review_request_sources(
            item, "reviewer", ["blinklabs-io/core"]
        )

        self.assertEqual(sources, ["direct", "team:blinklabs-io/core"])

    def test_authenticated_team_lookup_paginates_and_filters_owner(self) -> None:
        first_page = [
            {
                "slug": f"other-{index}",
                "organization": {"login": "another-org"},
            }
            for index in range(99)
        ]
        first_page.append(
            {"slug": "core", "organization": {"login": "blinklabs-io"}}
        )
        second_page = [
            {"slug": "docs", "organization": {"login": "blinklabs-io"}}
        ]

        with mock.patch.object(
            scan_prs, "gh_json", side_effect=[first_page, second_page]
        ) as gh_json:
            teams = scan_prs.authenticated_review_teams("blinklabs-io")

        self.assertEqual(teams, ["blinklabs-io/core", "blinklabs-io/docs"])
        self.assertEqual(gh_json.call_count, 2)

    def test_inspect_pr_reports_assignees(self) -> None:
        # A sweep self-assigns what it reviews so a teammate can see the work is
        # taken, and skips anything already assigned to someone else. Both rules
        # need the assignee list in the scan output.
        pr = {
            "repository": {"nameWithOwner": "blinklabs-io/example"},
            "number": 17,
            "title": "Example",
            "url": "https://github.com/blinklabs-io/example/pull/17",
            "author": {"login": "author"},
            "updatedAt": "2026-01-01T00:00:00Z",
        }
        responses = {
            "repos/blinklabs-io/example/pulls/17": {
                "head": {"sha": "abc123"},
                "mergeable_state": "clean",
                "requested_reviewers": [],
                "requested_teams": [],
                "assignees": [{"login": "reviewer-one"}, {"login": "reviewer-two"}],
            },
            "repos/blinklabs-io/example/pulls/17/reviews?per_page=100": [],
            "repos/blinklabs-io/example/issues/17/comments?per_page=100": [],
            "repos/blinklabs-io/example/commits/abc123/check-runs?per_page=100": {
                "check_runs": []
            },
        }

        def fake_gh_json(args: list[str]) -> object:
            return responses[args[1]]

        with mock.patch.object(scan_prs, "gh_json", fake_gh_json):
            item = scan_prs.inspect_pr(pr)
        self.assertEqual(item["assignees"], ["reviewer-one", "reviewer-two"])

    def test_inspect_pr_reports_empty_assignees(self) -> None:
        pr = {
            "repository": {"nameWithOwner": "blinklabs-io/example"},
            "number": 18,
            "title": "Example",
            "url": "https://github.com/blinklabs-io/example/pull/18",
            "author": {"login": "author"},
            "updatedAt": "2026-01-01T00:00:00Z",
        }
        responses = {
            "repos/blinklabs-io/example/pulls/18": {
                "head": {"sha": "def456"},
                "mergeable_state": "clean",
                "requested_reviewers": [],
                "requested_teams": [],
            },
            "repos/blinklabs-io/example/pulls/18/reviews?per_page=100": [],
            "repos/blinklabs-io/example/issues/18/comments?per_page=100": [],
            "repos/blinklabs-io/example/commits/def456/check-runs?per_page=100": {
                "check_runs": []
            },
        }

        def fake_gh_json(args: list[str]) -> object:
            return responses[args[1]]

        with mock.patch.object(scan_prs, "gh_json", fake_gh_json):
            item = scan_prs.inspect_pr(pr)
        self.assertEqual(item["assignees"], [])

    def _inspect(self, number: int, head: str, checks: list) -> dict:
        pr = {
            "repository": {"nameWithOwner": "blinklabs-io/example"},
            "number": number,
            "title": "Example",
            "url": f"https://github.com/blinklabs-io/example/pull/{number}",
            "author": {"login": "author"},
            "updatedAt": "2026-01-01T00:00:00Z",
        }
        responses = {
            f"repos/blinklabs-io/example/pulls/{number}": {
                "head": {"sha": head},
                "mergeable_state": "clean",
                "requested_reviewers": [],
                "requested_teams": [],
                "assignees": [],
            },
            f"repos/blinklabs-io/example/pulls/{number}/reviews?per_page=100": [],
            f"repos/blinklabs-io/example/issues/{number}/comments?per_page=100": [],
            f"repos/blinklabs-io/example/commits/{head}/check-runs?per_page=100": {
                "check_runs": checks
            },
        }
        with mock.patch.object(scan_prs, "gh_json", lambda args: responses[args[1]]):
            return scan_prs.inspect_pr(pr)

    def test_checks_completed_at_uses_latest_across_all_runs(self) -> None:
        # A re-review sweep asks "did CI finish anything since I last reviewed?".
        # That has to consider passing runs too: a pipeline going red to green
        # leaves no entry in problem_checks at all, so a problem-only timestamp
        # would report no change on exactly the transition worth revisiting.
        item = self._inspect(21, "abc123", [
            {"name": "lint", "status": "completed", "conclusion": "success",
             "completed_at": "2026-01-02T10:00:00Z"},
            {"name": "test", "status": "completed", "conclusion": "success",
             "completed_at": "2026-01-02T12:30:00Z"},
        ])
        self.assertEqual(item["problem_checks"], [])
        self.assertEqual(item["checks_completed_at"], "2026-01-02T12:30:00Z")

    def test_problem_checks_carry_completed_at(self) -> None:
        item = self._inspect(22, "def456", [
            {"name": "lint", "status": "completed", "conclusion": "failure",
             "completed_at": "2026-01-02T09:00:00Z"},
        ])
        self.assertEqual(item["problem_checks"][0]["completed_at"],
                         "2026-01-02T09:00:00Z")
        self.assertEqual(item["checks_completed_at"], "2026-01-02T09:00:00Z")

    def test_checks_completed_at_empty_while_running(self) -> None:
        item = self._inspect(23, "ghi789", [
            {"name": "test", "status": "in_progress", "conclusion": None},
        ])
        self.assertEqual(item["checks_completed_at"], "")

    def test_text_report_includes_team_review_request(self) -> None:
        item = result(review_request_sources=["team:blinklabs-io/core"])
        output = io.StringIO()

        with contextlib.redirect_stdout(output):
            scan_prs.print_text([item], "reviewer")

        self.assertIn("DIRECT OR TEAM REVIEW REQUESTS FOR reviewer", output.getvalue())
        self.assertIn("team:blinklabs-io/core", output.getvalue())


if __name__ == "__main__":
    unittest.main()
