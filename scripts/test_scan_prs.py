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

    def test_text_report_includes_team_review_request(self) -> None:
        item = result(review_request_sources=["team:blinklabs-io/core"])
        output = io.StringIO()

        with contextlib.redirect_stdout(output):
            scan_prs.print_text([item], "reviewer")

        self.assertIn("DIRECT OR TEAM REVIEW REQUESTS FOR reviewer", output.getvalue())
        self.assertIn("team:blinklabs-io/core", output.getvalue())


if __name__ == "__main__":
    unittest.main()
