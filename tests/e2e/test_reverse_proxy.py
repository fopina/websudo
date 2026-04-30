#!/usr/bin/env python3

import unittest
from pathlib import Path

from github_e2e import (
    PLACEHOLDER_ALLOW_ALL,
    PLACEHOLDER_BLOCK_ISSUE_2,
    GitHubE2ETestCase,
    issue_url,
    reverse_url,
)


class GitHubReverseProxyIssuesE2ETest(GitHubE2ETestCase):
    config_template = Path(__file__).with_name("configs") / "reverse_proxy.websudo.template.yaml"

    def test_placeholder_allow_all_injects_auth(self) -> None:
        self.assert_auth_response(
            PLACEHOLDER_ALLOW_ALL,
            reverse_url(self.addr, self.auth_check["url"]),
            "reverse",
        )

    def test_placeholder_block_issue_2_injects_auth(self) -> None:
        self.assert_auth_response(
            PLACEHOLDER_BLOCK_ISSUE_2,
            reverse_url(self.addr, self.auth_check["url"]),
            "reverse",
        )

    def test_placeholder_allow_all_can_access_both_issues(self) -> None:
        self.assert_url_status(PLACEHOLDER_ALLOW_ALL, reverse_url(self.addr, issue_url(1)), 200, "reverse-issue-1")
        self.assert_url_status(PLACEHOLDER_ALLOW_ALL, reverse_url(self.addr, issue_url(2)), 200, "reverse-issue-2")

    def test_placeholder_block_issue_2_denies_issue_2(self) -> None:
        self.assert_url_status(PLACEHOLDER_BLOCK_ISSUE_2, reverse_url(self.addr, issue_url(1)), 200, "reverse-issue-1")
        self.assert_url_status(PLACEHOLDER_BLOCK_ISSUE_2, reverse_url(self.addr, issue_url(2)), 403, "reverse-issue-2")


if __name__ == "__main__":
    unittest.main(verbosity=2)
