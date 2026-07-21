#!/usr/bin/env python3
"""Security regression tests for the loopback integration fixture."""

from __future__ import annotations

import base64
import http.client
import json
import os
import queue
import subprocess
import sys
import tempfile
import threading
import unittest
import urllib.error
import urllib.request
from pathlib import Path
from urllib.parse import urlsplit


ROOT = Path(__file__).resolve().parents[1]
FIXTURE = ROOT / "integration" / "fixture_api.py"
AUTHORIZATION = "Basic " + base64.b64encode(b"fixture-access:fixture-secret").decode()


class FixtureProcess:
    def __init__(self) -> None:
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.work_root = Path(self.temporary_directory.name)
        self.legacy_url_path = self.work_root / "legacy-url-must-not-exist"
        self.legacy_received_path = self.work_root / "legacy-payload-must-not-exist"
        environment = os.environ.copy()
        environment["FIXTURE_URL_PATH"] = str(self.legacy_url_path)
        environment["FIXTURE_RECEIVED_PATH"] = str(self.legacy_received_path)
        self.process = subprocess.Popen(
            [sys.executable, str(FIXTURE)],
            cwd=ROOT,
            env=environment,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        assert self.process.stdout is not None
        lines: queue.Queue[str] = queue.Queue(maxsize=1)
        reader = threading.Thread(
            target=lambda: lines.put(self.process.stdout.readline()), daemon=True
        )
        reader.start()
        try:
            self.url = lines.get(timeout=5).strip()
        except queue.Empty as error:
            self.process.terminate()
            self.process.wait(timeout=5)
            self.process.stdout.close()
            assert self.process.stderr is not None
            self.process.stderr.close()
            self.temporary_directory.cleanup()
            raise AssertionError("fixture did not publish its loopback URL on stdout") from error
        if not self.url.startswith("http://127.0.0.1:"):
            self.close()
            raise AssertionError(f"fixture published an unsafe URL: {self.url!r}")

    def close(self) -> None:
        self.process.terminate()
        try:
            self.process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.process.kill()
            self.process.wait(timeout=5)
        assert self.process.stdout is not None
        assert self.process.stderr is not None
        self.process.stdout.close()
        self.process.stderr.close()
        self.temporary_directory.cleanup()

    def request(
        self,
        path: str,
        *,
        data: bytes | None = None,
        authorized: bool = False,
    ) -> tuple[int, bytes]:
        request = urllib.request.Request(self.url + path, data=data)
        if data is not None:
            request.add_header("Content-Type", "application/json")
        if authorized:
            request.add_header("Authorization", AUTHORIZATION)
        try:
            with urllib.request.urlopen(request, timeout=5) as response:
                return response.status, response.read()
        except urllib.error.HTTPError as error:
            return error.code, error.read()


class FixtureSecurityTests(unittest.TestCase):
    def setUp(self) -> None:
        self.fixture = FixtureProcess()

    def tearDown(self) -> None:
        self.fixture.close()

    def test_legacy_output_paths_are_ignored_and_memory_flow_works(self) -> None:
        self.assertFalse(self.fixture.legacy_url_path.exists())
        self.assertFalse(self.fixture.legacy_received_path.exists())

        status, _ = self.fixture.request("/received", authorized=True)
        self.assertEqual(status, 404)

        payload = json.dumps({"proof": "memory-only"}).encode()
        status, _ = self.fixture.request("/ingest", data=payload)
        self.assertEqual(status, 204)
        status, received = self.fixture.request("/received", authorized=True)
        self.assertEqual(status, 200)
        self.assertEqual(received, payload)

        self.assertFalse(self.fixture.legacy_url_path.exists())
        self.assertFalse(self.fixture.legacy_received_path.exists())

    def test_received_payload_requires_fixture_credentials(self) -> None:
        status, _ = self.fixture.request("/ingest", data=b'{"value":1}')
        self.assertEqual(status, 204)
        status, _ = self.fixture.request("/received")
        self.assertEqual(status, 401)

    def test_invalid_or_oversized_payload_is_rejected(self) -> None:
        status, _ = self.fixture.request("/ingest", data=b"not-json")
        self.assertEqual(status, 400)
        target = urlsplit(self.fixture.url)
        connection = http.client.HTTPConnection(target.hostname, target.port, timeout=5)
        try:
            connection.request(
                "POST",
                "/ingest",
                body=b"",
                headers={"Content-Length": str((1024 * 1024) + 1)},
            )
            response = connection.getresponse()
            self.assertEqual(response.status, 413)
            response.read()
        finally:
            connection.close()


if __name__ == "__main__":
    unittest.main()
