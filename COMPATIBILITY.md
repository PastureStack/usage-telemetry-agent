# Compatibility Contract

The migration preserves the runtime surface used by the v1.6 launcher:

- the `client` subcommand;
- `CATTLE_URL`, `CATTLE_ACCESS_KEY`, and `CATTLE_SECRET_KEY` as secondary environment aliases;
- `TELEMETRY_LISTEN` and `TELEMETRY_INTERVAL` as secondary environment aliases;
- `GET /v1-telemetry`, `POST /v1-telemetry/reload`, and `POST /v1-telemetry/report`;
- loopback port `8114`; and
- the record version and high-level `install`, `environment`, `host`, `container`, `service`, and `stack` keys.

The `CATTLE_*` and `/v1-telemetry` strings are inherited wire identifiers, not PastureStack product identity. New configuration uses `PASTURESTACK_API_*` and `PASTURESTACK_USAGE_TELEMETRY_*` names. The neutral executable is `usage-telemetry-agent`; a Server package may install an internal `telemetry` link until the preserved launcher is renamed.

Compatibility is deliberately constrained where old behavior conflicts with privacy or safety:

- `TELEMETRY_TO_URL` is ignored. Only `PASTURESTACK_USAGE_TELEMETRY_TARGET_URL` enables publishing.
- No installation identifier is created or written. An existing value is included only as a full SHA-256 digest after explicit opt-in.
- Arbitrary labels are reduced to fixed categories, and raw image coordinates are never returned.
- The default listener changed from all interfaces to `127.0.0.1`. Remote binding requires an explicit flag.
- The API URL must use HTTPS unless it identifies the loopback control-plane API; ambient HTTP proxies are never used for credentialed API requests.
- Redirects are refused for both API and publishing requests.
- The upstream v0.6 cluster collector, central receiver, database, dashboard, CSV converter, and Kubernetes deployment modes are not part of this v1.6 runtime contract.

Before release, validate the component against the exact Server API, launcher supervision, disabled and enabled publishing modes, master-node failover, restart behavior, shutdown, rollback, and the immutable release archive in an isolated VM.
