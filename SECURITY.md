# Security Policy

## Supported migration target

Security review covers the single PastureStack maintenance commit built from the preserved upstream boundary. It remains a migration candidate until the exact release archive passes the isolated-VM gates documented in [COMPATIBILITY.md](COMPATIBILITY.md).

## Runtime requirements

- Keep the aggregate endpoint on loopback. Use `--allow-remote-listen` only with a separately reviewed network and access-control boundary.
- Supply API credentials through the launcher environment, not command-line arguments or source files.
- Leave external publishing disabled unless an operator has approved a specific destination and its privacy terms.
- Use an HTTPS publishing target. Loopback HTTP support exists only for testing.
- Do not place credentials or real environment data in issues, logs, repositories, or release assets.
- Verify the GitHub Release asset and executable SHA-256 digests before installation.

The client refuses redirects, refuses browser-origin POST requests, coalesces concurrent report triggers, bypasses ambient proxies for credentialed API calls, caps API responses at 8 MiB, caps a published record at 1 MiB, applies request timeouts, limits HTTP headers, and maps arbitrary category values to `other`. These controls reduce risk but do not turn a remote listener into a public API.

The integration fixture keeps received payloads in memory, exposes them only through its authenticated loopback endpoint, and does not accept environment-selected output paths. Fixture payloads are limited to 1 MiB and must be valid JSON.

## Reporting

Use GitHub private vulnerability reporting for security findings. Do not include active credentials, production addresses, raw telemetry, or private resource identifiers in a public issue.
