#!/usr/bin/env python3

import unittest
from pathlib import Path

from github_e2e import (
    PLACEHOLDER_ALLOW_ALL,
    PLACEHOLDER_BLOCK_ISSUE_2,
    GitHubE2ETestCase,
    curl_status,
    issue_url,
    response_message,
)


class GitHubForwardProxyIssuesE2ETest(GitHubE2ETestCase):
    config_template = Path(__file__).with_name("configs") / "forward_proxy.websudo.template.yaml"

    def test_placeholder_allow_all_injects_auth(self) -> None:
        self.assert_forward_auth_response(PLACEHOLDER_ALLOW_ALL)

    def test_placeholder_block_issue_2_injects_auth(self) -> None:
        self.assert_forward_auth_response(PLACEHOLDER_BLOCK_ISSUE_2)

    def test_placeholder_token_fails_without_proxy_auth_injection(self) -> None:
        self.assert_direct_auth_check_status(PLACEHOLDER_ALLOW_ALL, 401)
        self.assert_direct_auth_check_status(PLACEHOLDER_BLOCK_ISSUE_2, 401)

    def test_placeholder_allow_all_can_access_both_issues(self) -> None:
        self.assert_forward_issue_status(PLACEHOLDER_ALLOW_ALL, 1, 200)
        self.assert_forward_issue_status(PLACEHOLDER_ALLOW_ALL, 2, 200)

    def test_placeholder_block_issue_2_denies_issue_2(self) -> None:
        self.assert_forward_issue_status(PLACEHOLDER_BLOCK_ISSUE_2, 1, 200)
        self.assert_forward_issue_status(PLACEHOLDER_BLOCK_ISSUE_2, 2, 403)

    def assert_forward_auth_response(self, placeholder: str) -> None:
        auth_check = self.auth_check
        url = auth_check["url"]
        response_file = self.tmpdir / f"forward-{auth_check['name']}-{placeholder_safe_name(placeholder)}.json"
        status = curl_status(self.addr, self.ca_cert, placeholder, url, response_file)

        self.assertEqual(200, status, response_message(url, placeholder, 200, status, response_file))
        self.assert_auth_response_body(response_file, auth_check)

    def assert_forward_issue_status(self, placeholder: str, issue: int, expected: int) -> None:
        url = issue_url(issue)
        response_file = self.tmpdir / f"forward-issue-{issue}-{expected}.json"
        status = curl_status(self.addr, self.ca_cert, placeholder, url, response_file)

        self.assertEqual(expected, status, response_message(url, placeholder, expected, status, response_file))


def placeholder_safe_name(placeholder: str) -> str:
    return "".join(char if char.isalnum() else "-" for char in placeholder)


if __name__ == "__main__":
    unittest.main(verbosity=2)
