#!/usr/bin/env python3
"""Loopback-only integration fixture for the Usage Telemetry Agent."""

from __future__ import annotations

import base64
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlsplit


ACCESS = "fixture-access"
SECRET = "fixture-secret"
EXPECTED_AUTH = "Basic " + base64.b64encode(f"{ACCESS}:{SECRET}".encode()).decode()
MAX_PAYLOAD_BYTES = 1024 * 1024
RECEIVED_LOCK = threading.Lock()
RECEIVED_PAYLOAD: bytes | None = None

SETTINGS = {
    "telemetry.uid": "sensitive-fixture-installation-id",
    "rancher.server.image": "ghcr.io/sensitive-fixture/private-image:v1.6.270",
    "rancher.server.version": "v1.6.270",
    "api.security.enabled": "true",
    "api.auth.provider.configured": "ldapconfig",
}

COLLECTIONS = {
    "projects": [{"id": "1a1", "name": "sensitive-fixture-project", "orchestration": "native"}],
    "hosts": [{
        "id": "1h1",
        "name": "sensitive-fixture-host",
        "state": "active",
        "agentIpAddress": "192.0.2.70",
        "info": {
            "cpuInfo": {"count": 2, "mhz": 2400, "cpuCoresPercentages": [10, 20]},
            "memoryInfo": {"memTotal": 4096, "memAvailable": 3072},
            "osInfo": {
                "kernelVersion": "6.8.0-sensitive-fixture",
                "operatingSystem": "Ubuntu 26.04 sensitive-fixture",
                "dockerVersion": "Docker version 29.4.2, build sensitive-fixture",
            },
        },
    }],
    "machines": [{"driver": "generic", "name": "sensitive-fixture-machine"}],
    "containers": [{"state": "running", "hostId": "1h1", "name": "sensitive-fixture-container"}],
    "services": [{"state": "active", "stackId": "1st1", "type": "service", "name": "sensitive-fixture-service"}],
    "stacks": [{"state": "active", "accountId": "1a1", "externalId": "catalog://sensitive-fixture"}],
}


class Handler(BaseHTTPRequestHandler):
    server_version = "PastureStackIntegrationFixture/1"

    def log_message(self, _format: str, *_args: object) -> None:
        return

    def send_json(self, status: int, value: object) -> None:
        payload = json.dumps(value, separators=(",", ":")).encode()
        self.send_payload(status, payload, "application/json")

    def send_payload(self, status: int, payload: bytes, content_type: str) -> None:
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        if payload:
            self.wfile.write(payload)

    def do_GET(self) -> None:  # noqa: N802 - HTTP handler method name
        path = urlsplit(self.path).path
        if path == "/ready":
            self.send_json(200, {"status": "ok"})
            return
        if path == "/received":
            if self.headers.get("Authorization") != EXPECTED_AUTH:
                self.send_json(401, {"error": "unauthorized"})
                return
            with RECEIVED_LOCK:
                payload = RECEIVED_PAYLOAD
            if payload is None:
                self.send_json(404, {"error": "not-received"})
                return
            self.send_payload(200, payload, "application/json")
            return
        if self.headers.get("Authorization") != EXPECTED_AUTH:
            self.send_json(401, {"error": "unauthorized"})
            return
        prefix = "/v2-beta/"
        if not path.startswith(prefix):
            self.send_json(404, {"error": "not-found"})
            return
        resource = path[len(prefix):]
        if resource.startswith("settings/"):
            name = resource[len("settings/"):]
            self.send_json(200, {"name": name, "value": SETTINGS.get(name, "")})
            return
        if resource in COLLECTIONS:
            self.send_json(200, {"data": COLLECTIONS[resource]})
            return
        self.send_json(404, {"error": "not-found"})

    def do_POST(self) -> None:  # noqa: N802 - HTTP handler method name
        global RECEIVED_PAYLOAD

        if urlsplit(self.path).path != "/ingest":
            self.send_json(404, {"error": "not-found"})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            self.send_json(400, {"error": "invalid-content-length"})
            return
        if length <= 0:
            self.send_json(400, {"error": "empty-payload"})
            return
        if length > MAX_PAYLOAD_BYTES:
            self.send_json(413, {"error": "payload-too-large"})
            return
        payload = self.rfile.read(length)
        try:
            json.loads(payload)
        except (UnicodeDecodeError, json.JSONDecodeError):
            self.send_json(400, {"error": "invalid-json"})
            return
        with RECEIVED_LOCK:
            RECEIVED_PAYLOAD = payload
        self.send_response(204)
        self.end_headers()


def main() -> None:
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    host, port = server.server_address
    print(f"http://{host}:{port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
