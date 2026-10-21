# Privacy Boundary

## Default behavior

External publishing is off by default. Starting the process without `PASTURESTACK_USAGE_TELEMETRY_TARGET_URL` creates no publishing client and sends no aggregate outside the local control plane. The HTTP aggregate endpoint listens on loopback by default.

The process reads the compatible API with the credentials supplied by its launcher. Credentials are used only for same-origin API requests, are never written to disk by this program, and are never included in an aggregate.

## Aggregate fields

The record may contain:

- UTC collection time and schema version;
- control-plane version reduced to a plain numeric release or `custom`;
- product image class reduced to `pasturestack`, `custom`, or `unknown`;
- authentication, orchestration, operating-system, kernel-major/minor, container-engine-version, machine-driver, and service-kind fixed categories;
- environment, host, container, service, and stack counts; and
- minimum, average, maximum, and total capacity or utilization aggregates.

It does not contain resource names, host names, DNS names, IP addresses, URLs, account identifiers, project identifiers, catalog identifiers, raw image repositories or tags, access keys, secret keys, secret payloads, log contents, or arbitrary custom category labels.

## Installation identifier

The existing `telemetry.uid` setting is not read unless `PASTURESTACK_USAGE_TELEMETRY_INCLUDE_INSTALLATION_ID=true` or `--include-installation-id` is set. When explicitly enabled, the output contains a full SHA-256 digest rather than the original value. The program never creates or updates the setting. The digest remains stable and can therefore allow correlation across reports; leave this option disabled unless that property is required and documented by the destination owner.

## Publishing destination

Only an explicit HTTPS target is accepted. Plain HTTP is permitted only for a loopback test target. Redirects are refused. The destination operator is responsible for consent, access control, retention, deletion, regional requirements, and its own privacy notice. PastureStack provides no default hosted telemetry destination.
