## Roadmap

The roadmap prioritizes reliability and trust before expanding the number of
acquisition sources or adding a graphical interface.

### Phase 1: predictable and recoverable core

- separate output formatting from confirmation with `--json`, `--yes`, and
  `--non-interactive`
- define exit codes and JSON statuses for no-op, cancellation, skipped work,
  partial failure, and failure
- report missing and unsupported update sources explicitly
- make color terminal-aware and honor `NO_COLOR`
- strengthen update and ID-change transactions, including rollback failure
  reporting and committed-with-warning outcomes
- prevent concurrent `aim` processes from silently losing state updates
- add explicit HTTP timeouts and GitHub rate-limit diagnostics
- add black-box CLI tests covering output, exit status, cancellation, and
  temporary XDG environments
- document configuration, support boundaries, and recovery basics

### Phase 2: trustworthy update engine

- introduce a common update-provider interface and implement `zsync`
- support local-file updates for development and controlled environments
- persist and display update verification policy, with optional digest or
  signature verification
- replace script-based self-update with verified, atomic binary replacement
- retain the previous version after an update and support pinning and rollback
- add GitHub authentication and improved rate-limit handling

### Phase 3: diagnosis and lifecycle completion

- expand `diagnose` checks and add `repair`, `clean`, and update history workflows
- detect missing files, broken records, orphaned artifacts, invalid update
  metadata, and stale staging files
- add dry-run support to destructive operations
- make common partial failures recoverable without manually editing `apps.json`

### Phase 4: broader acquisition and customization

- add direct HTTPS URL installation with optional SHA-256 verification
- support initial metadata overrides such as app ID, name, and icon
- discover and import existing AppImages
- support multiple update targets and include/exclude filters
- add GitLab or a generic release-feed adapter

### Phase 5: 1.0 readiness

- stabilize the JSON schema, exit-code contract, and persisted-state migrations
- publish signed releases, SBOMs, and build provenance
- smoke-test release archives and representative AppImages
- publish a support matrix, security policy, and recovery/release runbooks
- define compatibility, rollback, and corruption-recovery guarantees
