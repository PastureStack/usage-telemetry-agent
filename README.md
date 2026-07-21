# PastureStack Usage Telemetry Agent

> **PastureStack modification notice:** the current runtime tree was replaced after the preserved upstream boundary to restore only the privacy-reduced v1.6 client contract. The unchanged history remains authoritative for inherited work.

Usage Telemetry Agent provides a privacy-reduced, opt-in aggregate endpoint for the preserved v1.6 control-plane contract. It replaces the retired automatic reporting client with a small standard-library-only executable.

PastureStack is an independent community effort to preserve, audit, and modernize the Rancher 1.6 ecosystem. It is not affiliated with or endorsed by Rancher Labs or SUSE.

**Upstream:** [`rancher/telemetry`](https://github.com/rancher/telemetry). This GitHub fork preserves upstream history, authorship, dates, tags, and the Apache-2.0 license. PastureStack maintenance is consolidated into one commit after the latest preserved upstream boundary.

## Privacy model

External publishing is disabled unless both of these independent conditions are true:

1. the preserved control plane launches the optional component; and
2. an operator configures `PASTURESTACK_USAGE_TELEMETRY_TARGET_URL` explicitly.

The retired `TELEMETRY_TO_URL` variable is deliberately ignored, so an inherited configuration cannot silently publish to an old endpoint. Without an explicit target, the process exposes aggregates only on `127.0.0.1:8114` and never initiates a publishing request.

The payload contains counts, distributions, and fixed categories. It excludes resource names, host names, addresses, raw image coordinates, catalog identifiers, credentials, and secret values. An existing installation identifier is omitted by default and is never created or written. See [PRIVACY.md](PRIVACY.md) for the exact data boundary.

## Compatibility

The executable accepts the inherited `client` subcommand, API credential environment aliases, loopback HTTP routes, and high-level record schema needed by the preserved launcher. New deployments use the `PASTURESTACK_*` names. The Server archive installs the neutral `usage-telemetry-agent` binary and may provide an internal `telemetry` compatibility link.

The later upstream cluster collector/server pipeline is intentionally not bundled. PastureStack does not ship a central collector, database, dashboard, or hosted telemetry destination.

## Build and test

On Linux with Go 1.26.6, Python 3.14.6, `bash`, `tar`, `xz`, and `curl`:

```sh
make validate
make test
make build
make integration-test
```

`scripts/check-content-policy` rejects local profile paths, private-network addresses, personal mailbox domains, non-English public text, and legacy names outside the documented legal/compatibility boundary. Maintainers can supply additional local-only literal checks through `PASTURESTACK_PRIVATE_PATTERN_FILE`; that file must remain outside the repository.

The runtime has no third-party Go module dependencies. `make package` produces the deterministic flat GitHub Release asset `usage-telemetry-agent-0.4.1-linux-amd64.tar.xz`. The matching Server release verifies the asset SHA-256 digest before installation; operators do not need an artifact mirror.

See [COMPATIBILITY.md](COMPATIBILITY.md), [SECURITY.md](SECURITY.md), [ORIGIN.md](ORIGIN.md), [MODIFICATIONS.md](MODIFICATIONS.md), and [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md).

## License and attribution

The inherited project remains licensed under the [Apache License 2.0](LICENSE). Copyright and attribution for inherited work remain with their respective authors and contributors. PastureStack contributors claim authorship only for their own changes.
