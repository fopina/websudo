import json
import os
import re
import shutil
import signal
import socket
import subprocess
import tempfile
import time
import unittest
from pathlib import Path
from typing import List, Optional

DEFECTDOJO_BASE = "https://demo.defectdojo.org"
LOGIN_PATH = "/login"
PROFILE_PATH = "/api/v2/user_profile/"
DEFAULT_COOKIE_SECRET = "websudo-e2e-cookie-secret"


class DefectDojoE2ETestCase(unittest.TestCase):
    config_template: Path

    @classmethod
    def setUpClass(cls) -> None:
        if not shutil.which("curl"):
            raise unittest.SkipTest("curl is required for e2e tests")

        cls.username = os.environ.get("WEBSUDO_E2E_DEFECTDOJO_USERNAME")
        cls.password = os.environ.get("WEBSUDO_E2E_DEFECTDOJO_PASSWORD")
        if not cls.username or not cls.password:
            raise unittest.SkipTest(
                "WEBSUDO_E2E_DEFECTDOJO_USERNAME and WEBSUDO_E2E_DEFECTDOJO_PASSWORD are required for live DefectDojo e2e tests"
            )

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
            cls.config_template.read_text(encoding="utf-8").format(
                listen=cls.addr,
                ca_cert_path=cls.ca_cert,
                ca_key_path=cls.ca_key,
            ),
            encoding="utf-8",
        )

        with cls.log_path.open("w", encoding="utf-8") as log:
            env = os.environ.copy()
            env.setdefault("WEBSUDO_E2E_COOKIE_SECRET", DEFAULT_COOKIE_SECRET)
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

    def login_and_fetch_profile(self, mode: str) -> None:
        cookie_jar = self.tmpdir / f"{mode}.cookies.txt"
        login_page = self.tmpdir / f"{mode}.login.html"
        login_post = self.tmpdir / f"{mode}.login-post.html"
        profile_json = self.tmpdir / f"{mode}.profile.json"

        login_url = self.login_url(mode)
        status = curl_status(
            self.proxy_addr(mode),
            self.proxy_cacert(mode),
            login_url,
            login_page,
            cookie_jar,
        )
        self.assertEqual(200, status, login_page.read_text(encoding="utf-8", errors="replace"))

        login_html = login_page.read_text(encoding="utf-8")
        csrf = extract_csrf_token(login_html)
        self.assertTrue(csrf)
        self.assert_cookie_jar_has_encrypted_cookie(cookie_jar, "csrftoken")

        posted_username = "definitely-not-the-real-user"
        posted_password = "definitely-not-the-real-pass"
        status = curl_status(
            self.proxy_addr(mode),
            self.proxy_cacert(mode),
            login_url,
            login_post,
            cookie_jar,
            method="POST",
            referer=login_url,
            headers=["Content-Type: application/x-www-form-urlencoded"],
            data=(
                f"csrfmiddlewaretoken={csrf}&username={posted_username}&password={posted_password}&next=%2F"
            ),
        )
        self.assertEqual(302, status, login_post.read_text(encoding="utf-8", errors="replace"))
        self.assert_cookie_jar_has_encrypted_cookie(cookie_jar, "sessionid")

        profile_url = self.profile_url(mode)
        status = curl_status(
            self.proxy_addr(mode),
            self.proxy_cacert(mode),
            profile_url,
            profile_json,
            cookie_jar,
            headers=["Accept: application/json"],
        )
        self.assertEqual(200, status, profile_json.read_text(encoding="utf-8", errors="replace"))

        profile = json.loads(profile_json.read_text(encoding="utf-8"))
        self.assertEqual(self.username, profile.get("username"))
        self.assertIn("user_id", profile)

    def assert_cookie_jar_has_encrypted_cookie(self, cookie_jar: Path, cookie_name: str) -> None:
        cookie_value = cookie_value_from_jar(cookie_jar, cookie_name)
        self.assertIsNotNone(cookie_value, cookie_jar.read_text(encoding="utf-8", errors="replace"))
        self.assertTrue(cookie_value.startswith("wsenc:"), f"expected encrypted {cookie_name}, got {cookie_value}")

    def login_url(self, mode: str) -> str:
        raise NotImplementedError

    def profile_url(self, mode: str) -> str:
        raise NotImplementedError

    def proxy_addr(self, mode: str) -> Optional[str]:
        raise NotImplementedError

    def proxy_cacert(self, mode: str) -> Optional[Path]:
        raise NotImplementedError


def curl_status(
    proxy_addr: Optional[str],
    ca_cert: Optional[Path],
    url: str,
    response_file: Path,
    cookie_jar: Path,
    *,
    method: str = "GET",
    headers: Optional[List[str]] = None,
    data: Optional[str] = None,
    referer: Optional[str] = None,
) -> int:
    args = [
        "curl",
        "--silent",
        "--show-error",
        "--output",
        str(response_file),
        "--write-out",
        "%{http_code}",
        "--cookie-jar",
        str(cookie_jar),
        "--cookie",
        str(cookie_jar),
        "--request",
        method,
    ]
    if headers:
        for header in headers:
            args.extend(["--header", header])
    if referer:
        args.extend(["--referer", referer])
    if data is not None:
        args.extend(["--data", data])
    if proxy_addr is not None and ca_cert is not None:
        args.extend(["--proxy", f"http://{proxy_addr}", "--cacert", str(ca_cert)])
    args.append(url)

    result = subprocess.run(args, check=True, text=True, capture_output=True)
    return int(result.stdout)


def extract_csrf_token(body: str) -> str:
    match = re.search(r'name="csrfmiddlewaretoken" value="([^"]+)"', body)
    if not match:
        raise AssertionError("csrfmiddlewaretoken not found in login page")
    return match.group(1)


def cookie_value_from_jar(cookie_jar: Path, cookie_name: str) -> Optional[str]:
    for line in cookie_jar.read_text(encoding="utf-8", errors="replace").splitlines():
        if not line or line.startswith("#"):
            continue
        parts = line.split("\t")
        if len(parts) >= 7 and parts[5] == cookie_name:
            return parts[6]
    return None


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


def coverage_out_path(repo_root: Path) -> Optional[Path]:
    configured = os.environ.get("WEBSUDO_E2E_COVERAGE_OUT")
    if not configured:
        return None
    path = Path(configured)
    if not path.is_absolute():
        path = repo_root / path
    return path
