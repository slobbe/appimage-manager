# Decision: Non-GitHub update sources

## Status

Accepted for current roadmap: **metadata-only for now, with implementation deferred to a separate design**.

## Decision

`aim` will continue to preserve and display non-GitHub update source metadata, but `aim update` will only plan and apply updates from GitHub release sources today.

Current source behavior:

- `github`: supported for update checks and applied updates when a repository is configured or parsed from supported embedded GitHub release metadata.
- `zsync`: preserved from embedded update metadata and shown by `aim info`, but not downloaded, diffed, or applied by `aim update` yet.
- `local_file`: preserved when explicitly stored, but not used as an update transport by `aim update` yet.
- `unsupported`: preserved as raw metadata for transparency and future parsing improvements, but never applied by `aim update`.

## Rationale

The existing service workflow already has GitHub release lookup, asset selection, bounded downloads, staged integration, and rollback behavior. Non-GitHub transports need different trust and integrity rules, so treating preserved metadata as actionable today would imply support that does not exist.

Keeping non-GitHub metadata visible is still useful: it helps users understand what an AppImage advertises, avoids losing data during integration, and leaves room for future implementation without another storage migration.

## Security implications

Before `zsync` or `local_file` updates are implemented, they need a separate design covering at least:

- HTTPS/TLS and redirect policy for remote `.zsync` files and referenced payload URLs.
- Integrity verification using zsync metadata, checksums, size limits, and failure handling.
- Protection against path traversal, local file replacement surprises, and untrusted local paths.
- Version comparison and downgrade policy when metadata is missing or ambiguous.
- Rollback semantics equivalent to the current GitHub update path.

Until that design exists, `aim update` must skip `zsync`, `local_file`, and `unsupported` sources without error.

## Consequences

- User-facing docs and human-readable info output should say that only GitHub update sources are applied today.
- JSON output should continue to expose existing update source fields without adding presentation-only status text.
- Tests should encode that non-GitHub sources are preserved/skipped rather than treated as update candidates.

## Follow-up

If the project chooses to support non-GitHub updates later, create separate implementation plans for zsync and local-file update semantics, starting with transport security, integrity verification, rollback, and version comparison behavior.
