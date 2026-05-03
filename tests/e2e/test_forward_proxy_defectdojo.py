#!/usr/bin/env python3

import unittest
from pathlib import Path
from typing import Optional

from defectdojo_e2e import DEFECTDOJO_BASE, DefectDojoE2ETestCase, LOGIN_PATH, PROFILE_PATH


class DefectDojoForwardProxyE2ETest(DefectDojoE2ETestCase):
    config_template = Path(__file__).with_name("configs") / "forward_proxy_defectdojo.websudo.template.yaml"

    def test_login_rewrites_credentials_and_roundtrips_encrypted_cookies(self) -> None:
        self.login_and_fetch_profile("forward")

    def login_url(self, mode: str) -> str:
        return f"{DEFECTDOJO_BASE}{LOGIN_PATH}?next=/"

    def profile_url(self, mode: str) -> str:
        return f"{DEFECTDOJO_BASE}{PROFILE_PATH}"

    def proxy_addr(self, mode: str) -> Optional[str]:
        return self.addr

    def proxy_cacert(self, mode: str) -> Optional[Path]:
        return self.ca_cert


if __name__ == "__main__":
    unittest.main(verbosity=2)
