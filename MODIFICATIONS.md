# PastureStack Modifications

The single PastureStack maintenance commit after upstream `v0.6.2`:

- narrows the runtime to the earlier v1.6 launcher and aggregate endpoint contract;
- replaces automatic reporting with two explicit opt-in gates and no default destination;
- removes the central receiver, database, dashboard, CSV converter, Kubernetes deployment, and obsolete CI/release plumbing from the current tree;
- removes third-party runtime dependencies and builds with the Go 1.26.6 standard library;
- changes product-owned package paths, executable identity, documentation, and archive names to PastureStack naming;
- retains old wire identifiers only inside documented compatibility and legal boundaries;
- prevents raw names, addresses, image coordinates, arbitrary labels, credentials, and secrets from entering aggregate records;
- adds same-origin pagination enforcement, redirect refusal, response-size limits, timeouts, unit tests, an executable integration test, and deterministic GitHub Release packaging.
- keeps integration payloads in a credential-protected loopback memory endpoint so external environment values cannot select filesystem write destinations.

Original authorship remains in the preserved Git history. PastureStack claims only the changes in its maintenance commit.
