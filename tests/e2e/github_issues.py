#!/usr/bin/env python3

import json
import os
import shutil
import signal
import socket
import subprocess
import tempfile
import time
import unittest
from pathlib import Path


PLACEHOLDER_ALLOW_ALL = "Bearer ph_allow_all"
PLACEHOLDER_BLOCK_ISSUE_2 = "Bearer ph_block_issue_2"
CONFIG_TEMPLATE = Path(__file__).with_name("github_issues.websudo.template.yaml")
AUTH_CHECKS = [
    {
        "name": "installation-repositories",
        "url": "https://api.github.com/installation/repositories",
        "validate": lambda body: any(
            repo.get("full_name") == "fopina/websudo"
            for repo in body.get("repositories", [])
        ),
        "failure": "Expected response to include the fopina/websudo installation repository",
    },
    {
        "name": "user",
        "url": "https://api.github.com/user",
        "validate": lambda body: bool(body.get("login")),
        "failure": "Expected response to include an authenticated user login",
    },
]


class GitHubIssuesE2ETest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        github_auth = os.environ.get("WEBSUDO_E2E_GITHUB_AUTH")
        if not github_auth:
            raise unittest.SkipTest("WEBSUDO_E2E_GITHUB_AUTH is required for live GitHub e2e tests")
        if not shutil.which("curl"):
            raise unittest.SkipTest("curl is required for e2e tests")
        cls.auth_check = discover_auth_check(github_auth)

        cls.repo_root = Path(__file__).resolve().parents[2]
        cls.tmp = tempfile.TemporaryDirectory()
        cls.tmpdir = Path(cls.tmp.name)
        cls.websudo_bin = cls.tmpdir / "websudo"
        cls.config_path = cls.tmpdir / "websudo.yaml"
        cls.ca_cert = cls.tmpdir / "ca.pem"
        cls.ca_key = cls.tmpdir / "ca-key.pem"
        cls.log_path = cls.tmpdir / "websudo.log"
        cls.coverage_out = coverage_out_path(cls.repo_root)
        cls.cover_dir = cls.tmpdir / "coverage" if cls.coverage_out is not None else None
        if cls.cover_dir is not None:
            cls.cover_dir.mkdir()
        cls.addr = f"127.0.0.1:{free_port()}"
        cls.proc = None

        build_cmd = ["go", "build", "-o", str(cls.websudo_bin), "."]
        if cls.coverage_out is not None:
            build_cmd[2:2] = ["-cover", "-covermode=atomic", "-coverpkg=./..."]
        subprocess.run(build_cmd, cwd=cls.repo_root, check=True)
        cls.config_path.write_text(
            CONFIG_TEMPLATE.read_text(encoding="utf-8").format(
                listen=cls.addr,
                ca_cert_path=cls.ca_cert,
                ca_key_path=cls.ca_key,
            ),
            encoding="utf-8",
        )

        with cls.log_path.open("w", encoding="utf-8") as log:
            env = os.environ.copy()
            if cls.cover_dir is not None:
                env["GOCOVERDIR"] = str(cls.cover_dir)
            cls.proc = subprocess.Popen(
                [str(cls.websudo_bin), "serve", "--config", str(cls.config_path)],
                env=env,
                stdout=log,
                stderr=subprocess.STDOUT,
            )
        wait_for_tcp(cls.addr, cls.log_path)

    @classmethod
    def tearDownClass(cls) -> None:
        proc = getattr(cls, "proc", None)
        if proc is not None:
            proc.send_signal(signal.SIGINT)
            try:
                proc.wait(timeout=2)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait(timeout=2)

        cover_dir = getattr(cls, "cover_dir", None)
        coverage_out = getattr(cls, "coverage_out", None)
        if cover_dir is not None and coverage_out is not None:
            subprocess.run(
                ["go", "tool", "covdata", "textfmt", f"-i={cover_dir}", f"-o={coverage_out}"],
                cwd=cls.repo_root,
                check=True,
            )

        tmp = getattr(cls, "tmp", None)
        if tmp is not None:
            tmp.cleanup()

    def test_placeholder_allow_all_injects_auth(self) -> None:
        self.assert_proxy_injects_auth(PLACEHOLDER_ALLOW_ALL)

    def test_placeholder_block_issue_2_injects_auth(self) -> None:
        self.assert_proxy_injects_auth(PLACEHOLDER_BLOCK_ISSUE_2)

    def test_placeholder_token_fails_without_proxy_auth_injection(self) -> None:
        self.assert_direct_auth_check_status(PLACEHOLDER_ALLOW_ALL, 401)
        self.assert_direct_auth_check_status(PLACEHOLDER_BLOCK_ISSUE_2, 401)

    def test_placeholder_allow_all_can_access_both_issues(self) -> None:
        self.assert_issue_status(PLACEHOLDER_ALLOW_ALL, 1, 200)
        self.assert_issue_status(PLACEHOLDER_ALLOW_ALL, 2, 200)

    def test_placeholder_block_issue_2_denies_issue_2(self) -> None:
        self.assert_issue_status(PLACEHOLDER_BLOCK_ISSUE_2, 1, 200)
        self.assert_issue_status(PLACEHOLDER_BLOCK_ISSUE_2, 2, 403)

    def assert_proxy_injects_auth(self, placeholder: str) -> None:
        auth_check = self.auth_check
        url = auth_check["url"]
        response_file = self.tmpdir / f"{auth_check['name']}-{safe_name(placeholder)}.json"
        status = curl_status(self.addr, self.ca_cert, placeholder, url, response_file)

        self.assertEqual(200, status, response_message(url, placeholder, 200, status, response_file))

        body = json.loads(response_file.read_text(encoding="utf-8"))
        self.assertTrue(auth_check["validate"](body), f"{auth_check['failure']}:\n{json.dumps(body, indent=2)}")

    def assert_direct_auth_check_status(self, placeholder: str, expected: int) -> None:
        auth_check = self.auth_check
        url = auth_check["url"]
        response_file = self.tmpdir / f"direct-{auth_check['name']}-{safe_name(placeholder)}.json"
        status = curl_status(None, None, placeholder, url, response_file)

        self.assertEqual(expected, status, response_message(url, placeholder, expected, status, response_file))

    def assert_issue_status(self, placeholder: str, issue: int, expected: int) -> None:
        url = f"https://api.github.com/repos/fopina/websudo/issues/{issue}"
        response_file = self.tmpdir / f"issue-{issue}-{expected}.json"
        status = curl_status(self.addr, self.ca_cert, placeholder, url, response_file)

        self.assertEqual(expected, status, response_message(url, placeholder, expected, status, response_file))


def curl_status(addr: str | None, ca_cert: Path | None, placeholder: str, url: str, response_file: Path) -> int:
    args = [
        "curl",
        "--silent",
        "--show-error",
        "--output",
        str(response_file),
        "--write-out",
        "%{http_code}",
        "--header",
        f"Authorization: {placeholder}",
        "--header",
        "Accept: application/vnd.github+json",
        "--header",
        "User-Agent: websudo-e2e",
    ]
    if addr is not None and ca_cert is not None:
        args.extend([
            "--proxy",
            f"http://{addr}",
            "--cacert",
            str(ca_cert),
        ])
    args.append(url)

    result = subprocess.run(
        args,
        check=True,
        text=True,
        capture_output=True,
    )
    return int(result.stdout)


def discover_auth_check(github_auth: str) -> dict:
    failures = []
    with tempfile.TemporaryDirectory() as tmp:
        tmpdir = Path(tmp)
        for auth_check in AUTH_CHECKS:
            url = auth_check["url"]
            response_file = tmpdir / f"discover-{auth_check['name']}.json"
            status = curl_status(None, None, github_auth, url, response_file)
            if status != 200:
                failures.append(response_message(url, github_auth, 200, status, response_file))
                continue

            body = json.loads(response_file.read_text(encoding="utf-8"))
            if auth_check["validate"](body):
                return auth_check

            failures.append(f"{auth_check['failure']}:\n{json.dumps(body, indent=2)}")

    raise unittest.SkipTest("WEBSUDO_E2E_GITHUB_AUTH cannot call supported auth-check endpoints\n" + "\n".join(failures))


def response_message(url: str, placeholder: str, expected: int, status: int, response_file: Path) -> str:
    body = response_file.read_text(encoding="utf-8", errors="replace")
    return f"Expected {url} with {placeholder} to return {expected}, got {status}\n{body}"


def free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def wait_for_tcp(addr: str, log_path: Path) -> None:
    host, port = addr.rsplit(":", 1)
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline:
        try:
            with socket.create_connection((host, int(port)), timeout=0.1):
                return
        except OSError:
            time.sleep(0.1)

    log = log_path.read_text(encoding="utf-8", errors="replace") if log_path.exists() else ""
    raise AssertionError(f"websudo did not start on {addr}\n{log}")


def safe_name(value: str) -> str:
    return "".join(char if char.isalnum() else "-" for char in value)


def coverage_out_path(repo_root: Path) -> Path | None:
    configured = os.environ.get("WEBSUDO_E2E_COVERAGE_OUT")
    if not configured:
        return None
    path = Path(configured)
    if not path.is_absolute():
        path = repo_root / path
    return path


if __name__ == "__main__":
    unittest.main(verbosity=2)
