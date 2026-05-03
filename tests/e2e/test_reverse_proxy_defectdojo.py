#!/usr/bin/env python3

import unittest
from pathlib import Path
from typing import Optional

from defectdojo_e2e import DefectDojoE2ETestCase, LOGIN_PATH, PROFILE_PATH


class DefectDojoReverseProxyE2ETest(DefectDojoE2ETestCase):
    config_template = Path(__file__).with_name("configs") / "reverse_proxy_defectdojo.websudo.template.yaml"

    def test_login_rewrites_credentials_and_roundtrips_encrypted_cookies(self) -> None:
        self.login_and_fetch_profile("reverse")

    def login_url(self, mode: str) -> str:
        return f"http://{self.addr}/dojo{LOGIN_PATH}?next=/"

    def profile_url(self, mode: str) -> str:
        return f"http://{self.addr}/dojo{PROFILE_PATH}"

    def proxy_addr(self, mode: str) -> Optional[str]:
        return None

    def proxy_cacert(self, mode: str) -> Optional[Path]:
        return None


if __name__ == "__main__":
    unittest.main(verbosity=2)
